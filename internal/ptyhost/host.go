// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package ptyhost shares the local shell through a PTY and streams it to clients
// via the relay over WebSocket.
package ptyhost

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	gpty "github.com/aymanbagabas/go-pty"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// hostTermSize returns the visible host terminal size. The console size must be
// queried from the output (screen buffer) handle: on Windows the input handle is
// not a screen buffer and GetConsoleScreenBufferInfo fails on it, which would
// otherwise silently fall back to 80x24. Trying stdout first works on every
// platform, with stdin and a default as fallbacks.
func hostTermSize() (cols, rows int) {
	if c, r, err := term.GetSize(int(os.Stdout.Fd())); err == nil && c > 0 && r > 0 {
		return c, r
	}
	if c, r, err := term.GetSize(int(os.Stdin.Fd())); err == nil && c > 0 && r > 0 {
		return c, r
	}
	return 80, 24
}

// Run connects a host. The relay allocates the session code and PIN and returns
// them in the upgrade response; this host then displays them for clients to use.
func Run(server string) {
	log.SetOutput(os.Stderr)

	fd := int(os.Stdin.Fd())
	wsURL := server + "/ws?" + url.Values{"role": {"host"}}.Encode()
	log.Printf("[host] connecting")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Fatalf("[host] connect: %v", err)
	}
	defer ws.Close()

	code := resp.Header.Get("X-Termlink-Code")
	pin := resp.Header.Get("X-Termlink-Pin")
	if code == "" || pin == "" {
		log.Fatalf("[host] relay did not return a session code/pin")
	}
	printBanner(code, pin)

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		log.Fatalf("[host] raw mode: %v", err)
	}
	defer term.Restore(fd, oldState)

	ptmx, err := gpty.New()
	if err != nil {
		log.Fatalf("[host] pty create: %v", err)
	}
	defer ptmx.Close()

	cmd := ptmx.Command(shellPath())
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if err := cmd.Start(); err != nil {
		log.Fatalf("[host] pty start: %v", err)
	}

	// Use the host terminal size as the initial PTY size.
	cols, rows := hostTermSize()
	ptmx.Resize(cols, rows)

	// Keep the PTY sized to the host terminal (SIGWINCH on Unix; no-op on
	// Windows, where resize arrives via console events in startStdinForward).
	watchResize(ptmx)

	done := make(chan struct{}, 1)
	var ptyMu sync.Mutex

	// PTY → host stdout + WebSocket
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
				ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// host stdin → PTY (local operator input)
	// On Windows: uses ReadConsoleInputW to avoid ConPTY ESC buffering, and
	// handles WINDOW_BUFFER_SIZE_EVENT to keep PTY sized to the host terminal.
	// PTY size = host terminal size so the host display is always correct.
	startStdinForward(ptmx, &ptyMu, func(cols, rows int) {
		ptmx.Resize(cols, rows)
	})

	// WebSocket → PTY (binary = keyboard from client; text = control)
	go func() {
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				ptyMu.Lock()
				ptmx.Write(data)
				ptyMu.Unlock()
			}
			// Client resize requests are intentionally ignored: PTY size is
			// authoritative from the host terminal so the host display stays correct.
		}
	}()

	<-done
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)
	cmd.Wait()
}

// printBanner shows the relay-allocated session code and PIN so the operator can
// share them with clients. Written to stderr before the terminal enters raw mode.
func printBanner(code, pin string) {
	fmt.Fprintf(os.Stderr, "\r\n"+
		"  ╭──────────────────────────────╮\r\n"+
		"  │  termlink session ready      │\r\n"+
		"  │    CODE : %-6s             │\r\n"+
		"  │    PIN  : %-4s               │\r\n"+
		"  ╰──────────────────────────────╯\r\n"+
		"  client connects with:\r\n"+
		"    termlink client -code %s -pin %s\r\n\r\n",
		code, pin, code, pin)
}
