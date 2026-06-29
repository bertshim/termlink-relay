// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package ptyclient

import (
	"os"

	"github.com/gorilla/websocket"
)

// startStdinSend forwards raw stdin bytes to the host as binary frames.
func startStdinSend(send func(int, []byte) error) {
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if werr := send(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
}
