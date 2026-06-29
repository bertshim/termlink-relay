// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package relay

import (
	"log"
)

// wsMsg wraps a WebSocket message with its frame type.
type wsMsg struct {
	msgType int
	data    []byte
}

// codeRoom holds the single host and the clients sharing a code.
type codeRoom struct {
	host    *Conn
	clients map[*Conn]bool
}

// Hub routes messages between peers. All room state is owned by the Run
// goroutine and must not be accessed concurrently.
type Hub struct {
	rooms map[string]*codeRoom
	creds map[string]string // server-allocated code → pin

	register   chan *Conn
	unregister chan *Conn
	route      chan routeMsg
	alloc      chan chan [2]string // host registration: returns {code, pin}
	check      chan checkReq       // client auth: validate code+pin
}

type routeMsg struct {
	from *Conn
	msg  *wsMsg
}

// checkReq asks the hub whether a code+pin pair matches an allocated session.
type checkReq struct {
	code string
	pin  string
	resp chan bool
}

func newHub() *Hub {
	return &Hub{
		rooms:      make(map[string]*codeRoom),
		creds:      make(map[string]string),
		register:   make(chan *Conn, 16),
		unregister: make(chan *Conn, 16),
		route:      make(chan routeMsg, 512),
		alloc:      make(chan chan [2]string, 16),
		check:      make(chan checkReq, 16),
	}
}

// Run processes hub events until the process exits. Run it as a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.doRegister(c)
		case c := <-h.unregister:
			h.doUnregister(c)
		case rm := <-h.route:
			h.doRoute(rm)
		case resp := <-h.alloc:
			h.doAlloc(resp)
		case req := <-h.check:
			h.doCheck(req)
		}
	}
}

// doAlloc reserves a fresh, unique numeric code with a numeric PIN and returns
// them to the caller. The credentials live until the room ends (doUnregister).
func (h *Hub) doAlloc(resp chan [2]string) {
	var code string
	for {
		code = randCode()
		if _, taken := h.creds[code]; !taken {
			break
		}
	}
	pin := randPIN()
	h.creds[code] = pin
	resp <- [2]string{code, pin}
}

// doCheck reports whether a client's code+pin matches an allocated session.
func (h *Hub) doCheck(req checkReq) {
	pin, ok := h.creds[req.code]
	req.resp <- ok && pin == req.pin
}

func (h *Hub) doRegister(c *Conn) {
	room, ok := h.rooms[c.code]
	if !ok {
		room = &codeRoom{clients: make(map[*Conn]bool)}
		h.rooms[c.code] = room
	}
	if c.role == "host" {
		if room.host != nil {
			room.host.ws.Close() // kick stale host
		}
		room.host = c
		log.Printf("[hub] host joined   code=%s", c.code)
	} else {
		room.clients[c] = true
		log.Printf("[hub] client joined code=%s total=%d", c.code, len(room.clients))
	}
}

func (h *Hub) doUnregister(c *Conn) {
	room, ok := h.rooms[c.code]
	if !ok {
		return
	}
	if c.role == "host" {
		room.host = nil
		log.Printf("[hub] host left     code=%s", c.code)
		// close all clients when host disconnects
		for cl := range room.clients {
			if cl.ws != nil {
				cl.ws.Close()
			}
		}
	} else {
		delete(room.clients, c)
		log.Printf("[hub] client left   code=%s remaining=%d", c.code, len(room.clients))
	}
	// remove empty code
	if room.host == nil && len(room.clients) == 0 {
		delete(h.rooms, c.code)
		delete(h.creds, c.code) // free the server-allocated credentials
		log.Printf("[hub] code ended code=%s", c.code)
	}
}

func (h *Hub) doRoute(rm routeMsg) {
	room, ok := h.rooms[rm.from.code]
	if !ok {
		return
	}
	if rm.from.role == "host" {
		// host output → broadcast to all clients
		// Each client gets its own copy of the data so concurrent writePumps
		// never share a backing array (safe under gorilla's internal framing).
		for cl := range room.clients {
			select {
			case cl.send <- rm.msg:
			default:
				// Client can't keep up — closing is safer than dropping ANSI frames.
				// Dropped frames corrupt terminal state permanently for TUI apps.
				log.Printf("[hub] client slow, closing  code=%s", cl.code)
				cl.closeSend()
				delete(room.clients, cl)
			}
		}
	} else {
		// client input → forward to host
		if room.host == nil {
			return
		}
		select {
		case room.host.send <- rm.msg:
		default:
			// Host send buffer full — close the client that sent input.
			log.Printf("[hub] host send buffer full, closing client  code=%s", rm.from.code)
			rm.from.closeSend()
		}
	}
}
