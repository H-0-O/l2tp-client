# go-l2tp FD patch (for local clone)

To get a **real VPN** (ppp0 over L2TP), the client must pass the same UDP socket FD to both the go-l2tp library and the kernel PPPoL2TP API. Upstream `github.com/katalix/go-l2tp` does not support an external FD, so we use a **local clone** and apply the patch there.

## Setup

1. Clone go-l2tp next to this repo (or wherever you set the path in `go.mod` replace):
   ```bash
   cd /path/to/parent
   git clone https://github.com/katalix/go-l2tp.git
   cd l2tp-client   # this project
   ```
2. In `go.mod` the replace points to `../go-l2tp`. If your clone is elsewhere, edit the replace path.
3. Apply the FD patch to the clone:
   ```bash
   patch -p1 -d ../go-l2tp < docs/go-l2tp-external-fd.patch
   ```
   (If the patch does not apply cleanly, apply the same edits by hand; see “What the patch does” below.)
4. Build this project:
   ```bash
   make build
   ```

## What the patch does

- **TunnelConfig.Fd** (in `l2tp/const.go`): Optional socket FD. When `Fd > 0`, the tunnel uses this FD as the control/data socket instead of creating a new one. The **caller owns** the FD; the library does not close it (so the same FD can be used for PPPoL2TP).
- **controlPlane.ownFd** (in `l2tp/controlplane.go`): When true, `close()` closes the socket file. When false (external FD), `close()` does not close it.
- **newL2tpControlPlaneFromFD** (in `l2tp/controlplane.go`): Builds a control plane from an existing FD; sets `ownFd = false`.
- **newDynamicTunnel** (in `l2tp/l2tp_dynamic_tunnel.go`): If `cfg.Fd > 0`, uses `newL2tpControlPlaneFromFD(cfg.Fd, sal, sap)` and skips `bind()` (socket is already set up by the caller).

## Client behaviour

- The client creates a UDP socket, gets an `*os.File` via `conn.File()`, and passes `file.Fd()` in `TunnelConfig.Fd`.
- It keeps the `*os.File` open for the lifetime of the tunnel and closes it on disconnect so the FD stays valid for both the library and `createPPPSocket()`.
