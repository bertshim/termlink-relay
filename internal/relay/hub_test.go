// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestConn creates a minimal Conn for testing (no real WebSocket)
func newTestConn(code, role string) *Conn {
	return &Conn{
		send: make(chan *wsMsg, 16),
		code: code,
		role: role,
	}
}

func startTestHub() *Hub {
	h := newHub()
	go h.Run()
	time.Sleep(5 * time.Millisecond)
	return h
}

func TestHubHostToClients(t *testing.T) {
	h := startTestHub()
	host := newTestConn("sess1", "host")
	client := newTestConn("sess1", "client")

	h.register <- host
	h.register <- client
	time.Sleep(10 * time.Millisecond)

	// host sends output → client should receive it
	msg := &wsMsg{msgType: websocket.BinaryMessage, data: []byte("output")}
	h.route <- routeMsg{from: host, msg: msg}
	time.Sleep(10 * time.Millisecond)

	select {
	case got := <-client.send:
		if string(got.data) != "output" {
			t.Fatalf("expected 'output', got %q", got.data)
		}
	default:
		t.Fatal("client did not receive host output")
	}

	// host should NOT receive its own message
	select {
	case <-host.send:
		t.Fatal("host received its own message unexpectedly")
	default:
	}
}

func TestHubClientToHost(t *testing.T) {
	h := startTestHub()
	host := newTestConn("sess2", "host")
	client := newTestConn("sess2", "client")

	h.register <- host
	h.register <- client
	time.Sleep(10 * time.Millisecond)

	// client sends input → host should receive it
	msg := &wsMsg{msgType: websocket.BinaryMessage, data: []byte("keypress")}
	h.route <- routeMsg{from: client, msg: msg}
	time.Sleep(10 * time.Millisecond)

	select {
	case got := <-host.send:
		if string(got.data) != "keypress" {
			t.Fatalf("expected 'keypress', got %q", got.data)
		}
	default:
		t.Fatal("host did not receive client input")
	}
}

func TestHubNoHostForClient(t *testing.T) {
	h := startTestHub()
	client := newTestConn("sess3", "client")

	h.register <- client
	time.Sleep(10 * time.Millisecond)

	// message from client with no host → must not panic
	msg := &wsMsg{msgType: websocket.BinaryMessage, data: []byte("orphan")}
	h.route <- routeMsg{from: client, msg: msg}
	time.Sleep(10 * time.Millisecond)
	// no panic = pass
}

func TestHubCodeCleanup(t *testing.T) {
	h := startTestHub()
	host := newTestConn("sess4", "host")
	client := newTestConn("sess4", "client")

	h.register <- host
	h.register <- client
	time.Sleep(10 * time.Millisecond)

	// unregister client first (so host unregister won't try to close nil ws)
	h.unregister <- client
	time.Sleep(10 * time.Millisecond)
	h.unregister <- host
	time.Sleep(10 * time.Millisecond)

	// code should be gone; routing to it must not panic
	msg := &wsMsg{msgType: websocket.BinaryMessage, data: []byte("ghost")}
	h.route <- routeMsg{from: host, msg: msg}
	time.Sleep(10 * time.Millisecond)
	// no panic = pass
}

func TestSessionAuth(t *testing.T) {
	h := newHub()
	go h.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWS(h, w, r)
	}))
	defer srv.Close()

	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")

	// host registers → relay allocates a code+pin in the response headers
	host, resp, err := websocket.DefaultDialer.Dial(wsBase+"/ws?role=host", nil)
	if err != nil {
		t.Fatalf("host connect: %v", err)
	}
	defer host.Close()
	code := resp.Header.Get("X-Termlink-Code")
	pin := resp.Header.Get("X-Termlink-Pin")
	if code == "" || pin == "" {
		t.Fatalf("relay did not allocate code/pin: code=%q pin=%q", code, pin)
	}

	// wrong PIN → 401
	_, badResp, err := websocket.DefaultDialer.Dial(wsBase+"/ws?role=client&code="+code+"&pin=wrongpin", nil)
	if err == nil {
		t.Fatal("expected error with wrong PIN")
	}
	if badResp == nil || badResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got response: %v", badResp)
	}

	// unknown code → 401
	_, unknownResp, err := websocket.DefaultDialer.Dial(wsBase+"/ws?role=client&code=999999&pin="+pin, nil)
	if err == nil {
		t.Fatal("expected error with unknown code")
	}
	if unknownResp == nil || unknownResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown code, got response: %v", unknownResp)
	}

	// correct code+PIN → success
	c, _, err := websocket.DefaultDialer.Dial(wsBase+"/ws?role=client&code="+code+"&pin="+pin, nil)
	if err != nil {
		t.Fatalf("expected success with correct code/PIN: %v", err)
	}
	c.Close()
}

func TestMissingParams(t *testing.T) {
	h := newHub()
	go h.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWS(h, w, r)
	}))
	defer srv.Close()

	base := srv.URL

	cases := []struct {
		url  string
		want int
	}{
		{base + "?pin=pin123&role=client", http.StatusBadRequest},         // no code
		{base + "?code=s&role=client", http.StatusBadRequest},             // no pin
		{base + "?code=s&pin=pin123", http.StatusBadRequest},              // no role
		{base + "?code=s&pin=pin123&role=invalid", http.StatusBadRequest}, // bad role
	}

	for _, tc := range cases {
		resp, err := http.Get(tc.url)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.url, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("GET %s: want %d got %d", tc.url, tc.want, resp.StatusCode)
		}
	}
}
