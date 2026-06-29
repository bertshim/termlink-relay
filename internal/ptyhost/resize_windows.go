// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package ptyhost

import gpty "github.com/aymanbagabas/go-pty"

// watchResize is a no-op on Windows: SIGWINCH does not exist there, so resize
// events are delivered through console input in startStdinForward instead.
func watchResize(_ gpty.Pty) {}
