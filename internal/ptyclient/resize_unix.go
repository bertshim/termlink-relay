// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package ptyclient

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// watchResize notifies the host of client terminal size changes on SIGWINCH.
func watchResize(send func(int, []byte) error, fd int) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			if cols, rows, err := term.GetSize(fd); err == nil {
				msg, _ := json.Marshal(ctrlMsg{Type: "resize", Cols: cols, Rows: rows})
				send(websocket.TextMessage, msg)
			}
		}
	}()
}
