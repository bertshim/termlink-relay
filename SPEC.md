# termlink Relay Protocol — v1

This document defines the **connection protocol** for termlink. Anyone who follows this
specification can independently implement a compatible relay server, host, and client. The
`internal/relay` package in this repository is a **reference implementation** of the protocol,
not part of the specification itself.

The protocol treats the authentication credential as an **opaque value**. PIN is the reference
credential scheme; production deployments are free to replace it with another scheme (e.g.
tokens) — the relay does not interpret the internal structure of the credential.

---

## 1. Roles and topology

```
[Client] ──ws──▶ [Relay] ◀──ws── [Host]
```

| Role | Description |
|------|-------------|
| **Relay** | The server that connects host and client within the same *code* and relays bytes. It does not run a shell and does not interpret stream contents. |
| **Host** | The peer that registers its PTY (shell) with a code. Exactly **one** per code. |
| **Client** | A peer that connects to a code and uses the host's shell. **N** per code. |

---

## 2. Transport

- All communication runs over **WebSocket**.
- The relay exposes the following HTTP endpoints:

| Path | Method | Purpose |
|------|--------|---------|
| `/ws` | GET (Upgrade) | WebSocket connection for host/client |
| `/health` | GET | Health check. Body `ok`, status 200 |

- Maximum frame size: **64 KiB** (`65536` bytes). The connection is closed if exceeded.
- Keepalive: the relay sends a WebSocket Ping roughly every **50 seconds**, and if no Pong
  arrives within **60 seconds** it treats the connection as dead and closes it. (Exact timing
  is a recommended value, not mandated by the protocol.)

---

## 3. Handshake

A peer sends a WebSocket Upgrade request to `/ws`, identifying itself with the following
**query parameters**. The required parameters differ by role: the **relay allocates the
`code` and credential for the host** and returns them in the upgrade response, so the host
sends neither. The host then conveys them out-of-band (it displays them) to clients, which
must present both.

| Parameter | Host | Client | Value | Meaning |
|-----------|------|--------|-------|---------|
| `role` | ✅ | ✅ | `host` \| `client` | The peer's role. Any other value is rejected. |
| `code` | ⛔ (allocated) | ✅ | scheme-specific | Join code. The relay allocates it for the host; the client must echo it back. Reference scheme: 6-digit number. |
| *auth credential* | ⛔ (allocated) | ✅ | scheme-specific | Authentication credential. The relay allocates it for the host; the client must present it. Reference scheme: `pin=<4-digit number>`. |

On a successful **host** upgrade, the relay returns the allocated values in the response
headers:

| Response header | Meaning |
|-----------------|---------|
| `X-Termlink-Code` | The allocated join code. |
| `X-Termlink-Pin` | The allocated credential (reference scheme). |

Examples:

```
# Host: no code/pin sent; relay returns them as X-Termlink-Code / X-Termlink-Pin
GET /ws?role=host
# Client: presents the code+pin the host displayed
GET /ws?code=8542&role=client&pin=6401
```

### Validation order (reference relay)

For a **host**, the relay allocates a unique `code` and a credential, then upgrades and
registers the host, returning the allocated values in the response headers.

For a **client**, the relay validates the following **before** upgrading to WebSocket:

1. If `role` is not `host`/`client` → **400 Bad Request**
2. If `code` or the credential is empty → **400 Bad Request**
3. If the `code` is unknown or the credential does not match that code's allocation →
   **401 Unauthorized**
4. On success, upgrade to WebSocket and register the peer in the code room.

> Credential allocation and validation are scheme-specific and **must be performed
> server-side only**. The protocol only requires that the relay allocates a credential per
> host session and validates it against the presenting client.

---

## 4. Code / room model

- The relay maintains one room per `code` key. A room consists of one host plus N clients.
- **Host replacement:** if a new host registers for a code that already has a host, the
  existing host connection is closed and replaced by the new host.
- **Code teardown:** when the host disconnects, the relay **closes all client connections**
  in that room. A room with neither a host nor any clients is removed.
- A client disconnecting does not affect other peers.

---

## 5. Message framing

Two WebSocket frame types carry distinct semantics.

### 5.1 Binary frames — terminal byte stream

Carry raw terminal bytes (including ANSI escape sequences) verbatim.

| Direction | Content | Relay action |
|-----------|---------|--------------|
| host → relay | PTY output (stdout) | **Broadcast to all clients** in the code |
| client → relay | keyboard input (stdin) | **Forward to the host** of the code. Dropped if there is no host. |

The relay neither interprets nor modifies the byte content.

### 5.2 Text frames — JSON control messages

A UTF-8 JSON object, discriminated by the `type` field.

```jsonc
{ "type": "hello",  "cols": 120, "rows": 40 }  // sent once by the client right after connecting
{ "type": "resize", "cols": 100, "rows": 30 }  // sent by the client whenever its terminal size changes
```

| type | Fields | Sender | Meaning |
|------|--------|--------|---------|
| `hello` | `cols`, `rows` | client | Reports the client's terminal size right after connecting |
| `resize` | `cols`, `rows` | client | Notifies a terminal size change |

> **Host PTY sizing policy (reference implementation):** the reference host keeps the PTY size
> **authoritative to the host's local terminal** and intentionally ignores the client's `resize`
> requests (so the host's local display is always correct). The control message types are
> defined by the protocol, but how a host applies them is an implementation policy.

Unknown `type` values must be ignored (forward compatibility).

---

## 6. Flow control / backpressure

Dropping frames from a terminal stream **permanently corrupts TUI screen state.** Therefore the
reference relay **does not drop frames** for a slow peer; it **closes the connection** instead.

- If a client cannot keep up with host output (send buffer overflow) → that **client connection
  is closed**.
- If the host's send buffer fills up → the **client that sent the input is closed**.

Other compatible implementations are encouraged to follow the same "close rather than drop"
principle.

---

## 7. Teardown

- On a clean shutdown, either side sends a WebSocket **Close** frame.
- Host shutdown → all clients are closed, per §4.
- The reference implementation waits briefly after Close to flush remaining frames before
  closing the socket.

---

## 8. Security scope of the reference relay (Non-normative)

The reference relay in this repository is a **minimal implementation meant to demonstrate the
protocol**, with the following limitations:

- Authentication is a **per-session code + PIN**, both allocated by the relay when a host
  registers (reference scheme: 6-digit code + 4-digit PIN). They live only for the lifetime of the room
  and are freed when it ends. The space is small, so the reference scheme is **not** intended
  to resist online guessing on a public relay — see the migration note below.
- It performs no WebSocket `Origin` check (all origins are allowed).
- It has **no operational features**: no accounts/identity, code ownership, billing, abuse
  prevention, or clustering.

The reference relay is therefore suitable for self-hosting, testing, and single-user use, and
**does not provide multi-tenant operational security.** To build a multi-user service, replace
the credential scheme of §3 with per-code token authentication and add Origin checks, rate
limiting, isolation, and so on. This operational layer is outside the scope of the protocol and
is the responsibility of each implementation.

---

## License

Copyright © 2026 Bert Shim &lt;bertshim@gmail.com&gt;

This specification and the accompanying reference implementation are licensed under the
[Apache License 2.0](./LICENSE).
