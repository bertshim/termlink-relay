// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package ptyhost

import (
	"os"
	"os/signal"
	"syscall"

	gpty "github.com/aymanbagabas/go-pty"
	"golang.org/x/term"
)

// watchResize resizes the PTY to match the host terminal whenever it changes,
// driven by SIGWINCH.
func watchResize(ptmx gpty.Pty) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
				ptmx.Resize(cols, rows)
			}
		}
	}()
}
