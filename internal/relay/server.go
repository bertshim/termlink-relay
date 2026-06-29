// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package relay implements the reference WebSocket relay that connects a host
// and its clients within a code. The relay allocates a numeric code and PIN per
// host session and validates clients against them.
package relay

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second
	maxMsgSize = 64 * 1024 // 64 KB per frame
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Conn represents a single WebSocket peer, either a host or a client.
type Conn struct {
	ws        *websocket.Conn
	send      chan *wsMsg // buffered outbound queue
	hub       *Hub
	code      string
	role      string // "host" or "client"
	closeOnce sync.Once
}

// closeSend closes the outbound queue exactly once, signalling writePump to stop.
func (c *Conn) closeSend() {
	c.closeOnce.Do(func() { close(c.send) })
}

// Run starts the relay server and blocks until a fatal error occurs.
// The relay allocates a fresh numeric code and PIN for every host session;
// there is no global, fixed PIN.
func Run(addr string) {
	log.Printf("[server] started  addr=%s  (per-session code/PIN)", addr)

	hub := newHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(hub, w, r)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[server] fatal: %v", err)
	}
}

// serveWS handles both roles. A host registers without credentials: the relay
// allocates a numeric code and PIN and returns them in the upgrade response
// headers (X-Termlink-Code / X-Termlink-Pin). A client must present a code and
// PIN matching an allocated session. After validation the request is upgraded
// to a WebSocket connection and the read/write pumps run.
func serveWS(h *Hub, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	role := q.Get("role")
	if role != "host" && role != "client" {
		http.Error(w, "role must be host or client", http.StatusBadRequest)
		return
	}

	var code string
	var respHeader http.Header

	if role == "host" {
		resp := make(chan [2]string, 1)
		h.alloc <- resp
		cp := <-resp
		code = cp[0]
		respHeader = http.Header{}
		respHeader.Set("X-Termlink-Code", cp[0])
		respHeader.Set("X-Termlink-Pin", cp[1])
	} else {
		pin := q.Get("pin")
		code = q.Get("code")
		if code == "" || pin == "" {
			http.Error(w, "missing code or pin", http.StatusBadRequest)
			return
		}
		resp := make(chan bool, 1)
		h.check <- checkReq{code: code, pin: pin, resp: resp}
		if !<-resp {
			http.Error(w, "invalid code or pin", http.StatusUnauthorized)
			return
		}
	}

	ws, err := upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		log.Printf("[server] upgrade error: %v", err)
		return
	}

	c := &Conn{
		ws:   ws,
		send: make(chan *wsMsg, 1024),
		hub:  h,
		code: code,
		role: role,
	}
	h.register <- c

	go c.writePump()
	c.readPump() // blocks until connection closes
}

// readPump reads incoming frames and forwards them to the hub for routing.
func (c *Conn) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.closeSend()
		c.ws.Close()
	}()

	c.ws.SetReadLimit(maxMsgSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		msgType, data, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[relay] read error code=%s role=%s: %v", c.code, c.role, err)
			}
			return
		}
		c.hub.route <- routeMsg{from: c, msg: &wsMsg{msgType: msgType, data: data}}
	}
}

// writePump drains the send queue and writes frames to the WebSocket,
// emitting periodic ping frames to keep the connection alive.
func (c *Conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// channel closed; send close frame
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteMessage(msg.msgType, msg.data); err != nil {
				return
			}

		case <-ticker.C:
			// keepalive ping to detect dead connections
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// randCode returns a random 6-digit numeric session code.
func randCode() string {
	return fmt.Sprintf("%06d", rand.Intn(1_000_000))
}

// randPIN returns a random 4-digit numeric PIN.
func randPIN() string {
	return fmt.Sprintf("%04d", rand.Intn(10_000))
}
