// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package ptyclient connects to a shared terminal code and bridges the local
// terminal to the relay over WebSocket.
package ptyclient

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

type ctrlMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// Run connects a client using PIN authentication.
func Run(server, code, pin string) {
	connect(server, code, url.Values{"pin": {pin}})
}

func connect(server, code string, auth url.Values) {
	log.SetOutput(os.Stderr)

	fd := int(os.Stdin.Fd())
	q := url.Values{"code": {code}, "role": {"client"}}
	for k, v := range auth {
		q[k] = v
	}
	wsURL := server + "/ws?" + q.Encode()
	log.Printf("[client] connecting  code=%s", code)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("[client] connect: %v", err)
	}
	defer ws.Close()
	log.Printf("[client] connected")

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		log.Fatalf("[client] raw mode: %v", err)
	}
	defer term.Restore(fd, oldState)

	var wsMu sync.Mutex
	wsSend := func(msgType int, data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return ws.WriteMessage(msgType, data)
	}

	// tell host our current terminal size so it resizes its PTY to match
	cols, rows, _ := term.GetSize(fd)
	hello, _ := json.Marshal(ctrlMsg{Type: "hello", Cols: cols, Rows: rows})
	wsSend(websocket.TextMessage, hello)

	// forward client terminal resize events to host PTY
	watchResize(wsSend, fd)

	done := make(chan struct{}, 1)

	// WebSocket → stdout
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				os.Stdout.Write(data)
			}
		}
	}()

	// stdin → WebSocket
	// On Windows uses ReadConsoleInputW to avoid VT escape sequence buffering
	// that causes lone \x1b (ESC) to be held indefinitely by os.Stdin.Read.
	startStdinSend(wsSend)

	<-done
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)
	log.Printf("[client] disconnected")
}
