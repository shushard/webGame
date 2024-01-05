package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub
}

func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close(websocket.StatusNormalClosure, "")
		close(c.send)
	}()

	for {
		var msg []byte
		_, msg, err := c.conn.Read(ctx)
		if err != nil {
			log.Printf("error: %v", err)
			break
		}
		log.Println(string(msg))
		msg = bytes.TrimSpace(bytes.Replace(msg, newline, space, -1))

		c.hub.broadcast <- msg
	}
}

func (c *Client) writePump(ctx context.Context) {
	defer func() {
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}
			err := c.conn.Write(ctx, 1, message)
			if err != nil {
				log.Printf("error: %v", err)
				return
			}
		}
	}
}

func (c *Client) messageSender(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)

	defer func() {
		ticker.Stop()
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	var i int
	for {
		select {
		case <-ticker.C:
			i++
			str := fmt.Sprintf("Hello, this is a message %d", i)
			message := []byte(str)
			c.send <- message
		case <-ctx.Done():
			return
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{conn: conn, hub: hub, send: make(chan []byte, 256)}
	hub.register <- client

	go client.writePump(context.Background())
	go client.readPump(context.Background())
	go client.messageSender(context.Background())
}
