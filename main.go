package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"nhooyr.io/websocket"
)

type game struct {
	ticks      int
	sampleJSON []byte
	subzPng    []byte
	send       chan []byte
	lastMsg    string
	conn       *websocket.Conn
}

const (
	screenWidth  = 320
	screenHeight = 240

	frameOX     = 0
	frameOY     = 0
	frameWidth  = 50
	frameHeight = 99
	frameCount  = 10
)

func websocketHandler(g *game) {
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}

	c, _, err := websocket.Dial(context.Background(), u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	for {
		_, msg, err := c.Read(context.Background())
		if err != nil {
			log.Fatalf("failed to read message: %v", err)
		}

		g.send <- msg
	}
}

var (
	runnerImage *ebiten.Image
)

func (g *game) Update() error {
	g.ticks++
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0, 64, 64, 255})
	select {
	case msg := <-g.send:
		g.lastMsg = string(msg)
	default:
	}
	x, y := g.ticks%640, g.ticks%360
	ebitenutil.DebugPrintAt(screen, "lastMsg: "+g.lastMsg, x, y)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(frameWidth)/2, -float64(frameHeight)/2)
	op.GeoM.Translate(screenWidth/2, screenHeight/2)
	i := (g.ticks / 5) % frameCount
	sx, sy := frameOX+i*frameWidth, frameOY
	screen.DrawImage(runnerImage.SubImage(image.Rect(sx, sy, sx+frameWidth, sy+frameHeight)).(*ebiten.Image), op)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	g := &game{
		send: make(chan []byte),
	}

	go websocketHandler(g)
	g.sampleJSON, _ = readFile("asset/sample.json")
	imgPath := filepath.Join("asset", "characters", "sub-zero", "subz.png")
	g.subzPng, _ = readFile(imgPath)
	img, _, err := image.Decode(bytes.NewReader(g.subzPng))
	if err != nil {
		log.Fatal(err)
	}
	runnerImage = ebiten.NewImageFromImage(img)
	ebiten.SetWindowSize(screenWidth*2, screenHeight*2)
	ebiten.SetWindowTitle("Animation (Ebitengine Demo)")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

// open opens a file. In a browser, it downloads the file via HTTP;
// otherwise, it reads the file on disk.
func open(name string) (io.ReadCloser, error) {
	name = filepath.Clean(name)
	if runtime.GOOS == "js" {
		// TODO: use more lightweight method such as marwan-at-work/wasm-fetch
		resp, err := http.Get(name)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}

	return os.Open(name)
}

func readFile(name string) ([]byte, error) {
	f, err := open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()

	return io.ReadAll(f)
}
