package main

import (
	"context"
	"log"
	"net/http"
	"time"

	engine "example.com/game/internal"
	"github.com/golang/protobuf/proto"
	"nhooyr.io/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	id   string
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) readPump(world *engine.World) {
	defer func() {
		event := &engine.Event{
			Type: engine.Event_type_exit,
			Data: &engine.Event_Exit{
				Exit: &engine.EventExit{PlayerId: c.id},
			},
		}
		message, err := proto.Marshal(event)
		if err != nil {
			log.Println(err)
		}
		world.HandleEvent(event)
		c.hub.broadcast <- message

		c.hub.unregister <- c
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), pongWait)
		defer cancel()

		_, message, err := c.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusGoingAway || websocket.CloseStatus(err) == websocket.StatusAbnormalClosure {
				log.Printf("error: %v", err)
			}
			break
		}
		c.hub.broadcast <- message // ?
		event := &engine.Event{}
		err = proto.Unmarshal(message, event)
		if err != nil {
			log.Println(err)
		}
		world.HandleEvent(event)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The hub closed the channel.
				c.conn.Close(websocket.StatusNormalClosure, "")
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), writeWait)
			defer cancel()

			err := c.conn.Write(ctx, websocket.MessageBinary, message)
			if err != nil {
				return
			}

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				err = c.conn.Write(ctx, websocket.MessageBinary, <-c.send)
				if err != nil {
					return
				}
			}

		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), writeWait)
			defer cancel()

			err := c.conn.Ping(ctx)
			if err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, world *engine.World, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	id := world.AddPlayer()
	client := &Client{id: id, hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	event := &engine.Event{
		Type: engine.Event_type_init,
		Data: &engine.Event_Init{
			Init: &engine.EventInit{
				PlayerId: id,
				Units:    world.Units,
			},
		},
	}
	message, err := proto.Marshal(event)
	if err != nil {
		//todo: remove unit
		log.Println(err)
	}

	// Create a new context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = conn.Write(ctx, websocket.MessageBinary, message)
	if err != nil {
		log.Println(err)
	}

	unit := world.Units[id]
	event = &engine.Event{
		Type: engine.Event_type_connect,
		Data: &engine.Event_Connect{
			Connect: &engine.EventConnect{Unit: unit},
		},
	}
	message, err = proto.Marshal(event)
	if err != nil {
		//todo: remove unit
		log.Println(err)
	}
	hub.broadcast <- message

	// Allow collection of memory referenced by the caller by doing all work
	// in new goroutines.
	go client.writePump()
	go client.readPump(world)
}
