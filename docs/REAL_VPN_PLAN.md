# Plan: Bring Real VPN to Life (PPP over L2TP)

**Status:** Client changes are implemented. Apply the go-l2tp patch after `go mod vendor` (see [VENDOR_PATCHES.md](VENDOR_PATCHES.md) and `go-l2tp-external-fd.patch`).

## Goal

Get a working VPN: persistent `ppp0` (or similar) interface, IP assigned by the server, and traffic routed over the L2TP tunnel. This requires the **tunnel socket FD** so we can create a PPPoL2TP socket and attach pppd to the real L2TP session.

## Why It Doesn’t Work Today

- go-l2tp creates the tunnel’s UDP socket internally and **does not expose** its file descriptor.
- The kernel PPPoL2TP API requires that **exact** socket FD when connecting the PPP socket to the L2TP session.
- Without the FD we fall back to `pppd pty "echo ..."`, which does not use the L2TP tunnel and the interface disappears when the command exits.

## High-Level Approach

1. **Vendor go-l2tp** and add support for using an **external** tunnel socket FD (config field + use it when building the transport).
2. **Keep our UDP socket alive** in the client and pass its FD into the vendored library so the **same** socket is used for both L2TP control and PPPoL2TP.
3. **Wire PPP to L2TP** using the existing `createPPPSocket()` path once we have a valid FD.

---

## Step 1: Vendor go-l2tp and Add FD Support

### 1.1 Vendor the module

```bash
go mod vendor
```

This copies `github.com/katalix/go-l2tp` (and deps) into `vendor/`.

### 1.2 Find where the tunnel creates the transport socket

In the vendored tree, the dynamic tunnel (L2TPv2 client) will create a UDP socket for the tunnel. We need to:

- Locate the constructor or config that builds the transport (e.g. `transport.go`, `l2tp_dynamic_tunnel.go`, or similar).
- Identify the type that holds the tunnel config (e.g. `TunnelConfig` or an internal config struct) and the code path that creates the UDP `net.PacketConn` (or socket) for the tunnel.

Search for:

- `DialUDP`, `ListenUDP`, `net.Dial`, or `socket` creation in the tunnel/transport code.
- Struct types used for tunnel configuration (e.g. `TunnelConfig`, `tunnelConfig`).

### 1.3 Add an optional FD field to the tunnel config

- In the **vendored** config struct used when creating the dynamic tunnel, add an optional field, e.g.:
  - `Fd int` (0 = “create socket myself”, > 0 = “use this FD”).
- Ensure this struct is the one passed through from the public API (e.g. `l2tp.TunnelConfig`) if that’s what we use when calling `NewDynamicTunnel`.

### 1.4 Use the FD when non-zero

In the code path that creates the tunnel’s transport:

- If `config.Fd > 0`:
  - Build a `net.PacketConn` (or the exact type the library uses) from that FD.
  - In Go, you can use `net.FilePacketConn(os.NewFile(uintptr(config.Fd), "udp"))` (or the socket type the library expects). Ensure the FD is not closed by our client when we pass it (see Step 2).
- If `config.Fd == 0`:
  - Keep existing behavior: create the UDP socket inside the library as today.

Important: the library must use this socket for **all** L2TP traffic (control and data) so that the same FD can be passed to the kernel for PPPoL2TP.

### 1.5 Document the patch

- Add a short comment in the vendored file(s) and/or in this repo (e.g. `docs/VENDOR_PATCHES.md`) describing:
  - “TunnelConfig now supports optional Fd. When set, the tunnel uses this FD as the transport socket instead of creating a new one.”
- So future `go mod vendor` or manual re-vendoring can re-apply the same idea.

---

## Step 2: Client: Keep Tunnel Socket Alive and Pass FD

Today `createTunnelSocket` returns an FD but then closes the `net.UDPConn` and the `*os.File`, so the returned FD is **closed** and must not be used. We need the FD to stay valid for the whole tunnel lifetime.

### 2.1 Keep the tunnel connection in the client

- Add a field to hold the tunnel’s UDP connection, e.g. `tunnelConn *net.UDPConn` (or `tunnelFile *os.File` if you prefer to pass an FD and keep the file open).
- In `Connect()`, **before** calling `NewDynamicTunnel`:
  - Create the UDP socket with the existing logic (resolve server, `DialUDP`).
  - Store the `*net.UDPConn` in `tunnelConn` (and do **not** close it in `createTunnelSocket`).
  - Get the FD: e.g. `file, err := conn.File()`, then use `int(file.Fd())`. You must **keep** either the `conn` or the `file` alive for the lifetime of the tunnel; otherwise the FD can be closed by GC or when the file is closed. Prefer keeping `tunnelConn` and calling `conn.File()` only when you need the FD (e.g. when calling `createPPPSocket`), or keep a single `*os.File` and use its FD.

### 2.2 Refactor createTunnelSocket

- Option A: Change it to return `(*net.UDPConn, error)`. The client stores the conn and, when it needs the FD for PPP, does `conn.File()` and uses `file.Fd()` (and keeps the file or conn open until disconnect).
- Option B: Keep returning `(int, error)` but do **not** close the conn/file inside `createTunnelSocket`; instead, store the conn (or file) in the client and close it only in `Disconnect`/`Close`.

Ensure the FD you pass to the library and later to `createPPPSocket` is the same FD that backs the tunnel’s UDP socket.

### 2.3 Pass FD into the vendored TunnelConfig

- When building `TunnelConfig` for `NewDynamicTunnel`, set the new field, e.g. `tunnelCfg.Fd = tunnelSock` (where `tunnelSock` is the FD from the socket you’re keeping alive).
- Remove the reflection-based FD logic for this field (you can keep reflection as fallback for unvendored builds if desired).
- Ensure the socket is **bound/connected** as the library expects (e.g. already connected to the server address). Our current `DialUDP` does that.

### 2.4 Cleanup on disconnect

- In `Disconnect()` (and in the “not connected” path in `Close()` when we close the tunnel), close the stored `tunnelConn` (or the `*os.File`) so the FD is closed and the kernel L2TP/PPP state can be torn down.

---

## Step 3: Use Existing createPPPSocket Path

Once the library uses our socket and we keep it open:

- `c.tunnelSock` will be a valid FD (the same one the library uses for the tunnel).
- After tunnel/session are up and we have tunnel/session IDs (already captured from the logger), we already call `createPPPSocket()` when `c.tunnelSock != -1`.
- `createPPPSocket()` already builds the PPPoL2TP address (with peer address and IDs), connects the PPP socket to the L2TP session, creates the PPP device, and returns the device name.
- We already pass that device name to the PPP manager so pppd uses the real device instead of the pty fallback.

So no change is required inside `createPPPSocket()` logic beyond ensuring:

- The FD we pass is the **same** socket the library uses (guaranteed if we pass it in via `TunnelConfig.Fd` and the library uses it).
- Peer IDs (`RemoteTunnelID`, `RemoteSessionID`) are set when the server sends them (we already parse from logs; if the library exposes them in config/events later, we can use that too).

---

## Step 4: Optional Improvements

- **Peer IDs**: If the server sends peer_tunnel_id / peer_session_id in a later log or event, parse and set `RemoteTunnelID` / `RemoteSessionID` so the kernel has correct peer IDs (some setups work with 0; having real values is better).
- **Default route**: Document or add an option to run pppd with `defaultroute` so the VPN gets the default route when desired.
- **Re-vendoring**: Document in `docs/VENDOR_PATCHES.md` exactly which files and which lines were changed so that after a `go get -u` and `go mod vendor`, the FD patch can be re-applied.

---

## File Checklist

| Item | Location |
|------|----------|
| Vendor go-l2tp | `go mod vendor` |
| Add `Fd` to tunnel config (vendored) | `vendor/github.com/katalix/go-l2tp/l2tp/...` (config struct) |
| Use FD when creating transport (vendored) | Same package, transport/tunnel creation path |
| Keep tunnel conn in client | `internal/client/client.go` (new field, e.g. `tunnelConn *net.UDPConn`) |
| Create socket and set FD in config | `internal/client/client.go` (`Connect`) |
| Close tunnel conn on disconnect/close | `internal/client/client.go` (`Disconnect`, `Close`) |
| Document vendored patches | `docs/VENDOR_PATCHES.md` (new file) |

---

## Testing

1. Build with vendored module: `go build -mod=vendor ./cmd/l2tp-client`.
2. Run: `sudo ./l2tp-client connect --config m.toml`.
3. Expect:
   - No “Tunnel socket FD not available” message.
   - “Set tunnel socket FD: …” or use of the new config field.
   - pppd started with the PPP device (e.g. `ppp0`) from `createPPPSocket()`.
   - `ip addr` shows `ppp0` (or similar) with an address after the connection is up.
4. Disconnect with Ctrl+C and confirm the process exits and the interface goes away.

---

## Summary

- **Vendor** go-l2tp and add **optional tunnel socket FD** in config and use it when building the transport.
- **Client**: keep the tunnel **UDP socket** (or its FD) alive, pass its FD into the vendored config, and close it only on disconnect/close.
- **Existing** `createPPPSocket()` and pppd wiring then provide the **real VPN** (real ppp0 over L2TP). No change to the kernel PPPoL2TP API usage is required beyond having a valid, shared FD.
