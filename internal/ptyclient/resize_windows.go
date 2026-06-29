// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package ptyclient

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// watchResize notifies the host of client terminal size changes. Windows has no
// SIGWINCH, so it polls the console size on a short interval.
func watchResize(send func(int, []byte) error, fd int) {
	go func() {
		prevCols, prevRows, _ := term.GetSize(fd)
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			cols, rows, err := term.GetSize(fd)
			if err == nil && (cols != prevCols || rows != prevRows) {
				prevCols, prevRows = cols, rows
				msg, _ := json.Marshal(ctrlMsg{Type: "resize", Cols: cols, Rows: rows})
				send(websocket.TextMessage, msg)
			}
		}
	}()
}
