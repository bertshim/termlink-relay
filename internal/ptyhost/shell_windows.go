// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package ptyhost

import "os"

func shellPath() string {
	// cmd.exe is the default: PSReadLine in PowerShell intercepts ESC inside
	// ConPTY even while a child process is running, swallowing the key before
	// it reaches the app (e.g. Claude Code). cmd.exe has no such interception.
	// Override with SHELL env var to use a different shell (e.g. bash, pwsh).
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "cmd.exe"
}
