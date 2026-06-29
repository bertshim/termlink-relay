#!/usr/bin/env bash
# Copyright 2026 Bert Shim <bertshim@gmail.com>
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
cd "$(dirname "$0")"
go build -o termlink ./cmd/termlink
echo "Built: $(pwd)/termlink"
