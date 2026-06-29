# termlink

A **single-binary** tool that relays a remote terminal over WebSocket.
One `termlink` binary runs in three modes: **server / host / client**.

```
[Client PC] ──ws──▶ [Relay Server] ◀──ws── [Host PC]
                   (public IP, :9000)
```

- **Host** — the machine that registers its shell (PTY) with the relay server. Linux / macOS only.
- **Client** — the machine that connects to the host's shell through the relay server. Linux / macOS.
- **Server** — the relay that connects host and client within the same code. Run it where a public IP is available.

Authentication is **per session**: when the host starts, the relay allocates a 6-digit
`code` and a 4-digit `pin` and the host displays them. The client connects by supplying that
same `-code` and `-pin`.

The relay in this repository is a **reference implementation of the connection protocol** —
anyone can implement and operate their own relay against the same protocol. See
[`SPEC.md`](./SPEC.md) for the full specification.

> ⚠️ **Security scope of the reference relay**
> The reference relay allocates a **per-session code + PIN** (6-digit code, 4-digit PIN) and performs
> no WebSocket `Origin` check (`CheckOrigin` always allows). The small credential space is
> fine for self-hosting, testing, and single-user use, but it **does not resist online
> guessing on a public relay and provides no multi-tenant security.** For a multi-user
> production deployment, replace the authentication layer (e.g. per-session token auth) as
> described in `SPEC.md`.

---

## Build

Requires Go 1.22 or newer. Works on macOS, Linux, and Windows.

```bash
# macOS / Linux
./build.sh

# Windows
build.bat

# or directly with the Go toolchain (any OS)
go build -o termlink ./cmd/termlink
```

This produces a `termlink` (`termlink.exe` on Windows) binary in the project root.

---

## Usage

Run the whole flow with three terminals.

```bash
# Terminal 1 — server
./termlink server -addr :9000

# Terminal 2 — host (Linux/macOS). The relay allocates a code + PIN, shown in a banner:
./termlink host -server ws://localhost:9000
#   ╭──────────────────────────────╮
#   │  termlink session ready      │
#   │    CODE : 481572             │
#   │    PIN  : 6401               │
#   ╰──────────────────────────────╯

# Terminal 3 — client (use the code + PIN the host displayed)
./termlink client -server ws://localhost:9000 -code 481572 -pin 6401
```

Type in terminal 3 → the shell runs in terminal 2 → its output appears in terminal 3.

> The **host needs no `-code`/`-pin`** — the relay allocates them and the host prints them.
> The client must supply that exact `-code` and `-pin`.

When connecting to a remote server, you can set an environment variable instead of passing
`-server` every time.

```bash
export TERMLINK_SERVER="ws://RELAY_IP:9000"
./termlink host                       # relay allocates and prints the code + PIN
./termlink client -code 481572 -pin 6401
```

---

## Commands / flags

### Subcommands

| Command | Description | Key flags |
|---------|-------------|-----------|
| `termlink server` | Run the relay server | `-addr` |
| `termlink host`   | Share this machine's terminal | `-server` |
| `termlink client` | Connect to a shared terminal | `-server` `-code` `-pin` |

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | `ws://localhost:9000` (or `TERMLINK_SERVER`) | relay URL, e.g. `ws://1.2.3.4:9000` |
| `-code` | (client, required) | session code shown by the host |
| `-pin` | (client, required) | session PIN shown by the host |
| `-addr` | `:9000` | server listen address |

> The host takes no `-code`/`-pin`: the relay allocates both (6-digit code, 4-digit PIN) per session.

### Legacy `-mode` style (same behavior)

```bash
termlink -mode server -addr :9000
termlink -mode host   -server ws://HOST:9000
termlink -mode client -server ws://HOST:9000 -code 481572 -pin 6401
```

In `-mode host`, only `-server` is required (the relay allocates the code + PIN). In
`-mode client`, all of `-server` `-code` `-pin` are **required**.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `TERMLINK_SERVER` | default relay URL for host/client |

### Server endpoints

| Path | Purpose |
|------|---------|
| `/ws` | host/client WebSocket connection (code relay) |
| `/health` | health check — `curl http://RELAY_IP:9000/health` |

---

## Behavior notes

- Resizing the client terminal window is propagated to the host PTY automatically.
- To exit: type `exit` (ends the shell) or drop the connection — it returns automatically.
- Host mode uses a PTY, so it is **Linux / macOS only** (server/client are unaffected).

---

## Author

**Bert Shim** &lt;bertshim@gmail.com&gt;
GitHub: [github.com/bertshim/termlink-relay](https://github.com/bertshim/termlink-relay)

Contributions, issues, and pull requests are welcome.

## License

Copyright © 2026 Bert Shim &lt;bertshim@gmail.com&gt;

Licensed under the [Apache License 2.0](./LICENSE). See [`NOTICE`](./NOTICE) for
attribution requirements. You may use, modify, and distribute this software in
compliance with the License.
