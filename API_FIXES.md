# API Fixes Applied

## Summary of Changes

The go-l2tp library API differs from initial assumptions. The following fixes have been applied:

### 1. Package Structure
- **Before**: Used separate `github.com/katalix/go-l2tp/config` package
- **After**: All types are in `github.com/katalix/go-l2tp/l2tp` package
- **Fix**: Removed `l2tpconfig` import, use `l2tp` package directly

### 2. Tunnel and Session Types
- **Before**: Stored as `*l2tp.Tunnel` and `*l2tp.Session` (pointers)
- **After**: Stored as `l2tp.Tunnel` and `l2tp.Session` (interfaces)
- **Fix**: Changed field types from pointers to interfaces

### 3. Tunnel Creation Method
- **Before**: Used `NewStaticTunnel` (L2TPv3 only)
- **After**: Use `NewDynamicTunnel` for L2TPv2 client mode
- **Fix**: Changed to `NewDynamicTunnel` which runs full control protocol

### 4. Configuration Types
- **Before**: `l2tpconfig.TunnelConfig` and `l2tpconfig.SessionConfig`
- **After**: `l2tp.TunnelConfig` and `l2tp.SessionConfig`
- **Fix**: Use types from `l2tp` package

### 5. Constants
- **Before**: `l2tpconfig.L2TPv2`, `l2tpconfig.UDPEncap`, `l2tpconfig.PPPPseudowire`
- **After**: `l2tp.ProtocolVersion2`, `l2tp.EncapTypeUDP`, `l2tp.PseudowireTypePPP`
- **Fix**: Updated constant names (may need verification)

### 6. ID Methods
- **Before**: `tunnel.ID()`, `session.ID()`
- **After**: IDs come from config: `tunnelCfg.TunnelID`, `sessionCfg.SessionID`
- **Fix**: Get IDs from configuration structs (for dynamic tunnels, IDs assigned during negotiation)

### 7. Close Methods
- **Before**: `tunnel.Close()` returns error
- **After**: `tunnel.Close()` is void (no return value)
- **Fix**: Removed error handling from Close() calls

## Remaining Considerations

### Event Handling
For production use, implement event handlers to:
- Receive `TunnelUpEvent` when tunnel is established
- Receive `SessionUpEvent` when session is established
- Get actual tunnel/session IDs from events
- Handle `TunnelDownEvent` and `SessionDownEvent` for reconnection

Example:
```go
type eventHandler struct {
    tunnelID, sessionID uint32
    interfaceName string
}

func (h *eventHandler) HandleEvent(event interface{}) {
    switch e := event.(type) {
    case *l2tp.TunnelUpEvent:
        h.tunnelID = uint32(e.Config.TunnelID)
    case *l2tp.SessionUpEvent:
        h.sessionID = uint32(e.SessionConfig.SessionID)
        h.interfaceName = e.InterfaceName
    }
}
```

### Configuration Field Names
The exact field names in `TunnelConfig` and `SessionConfig` may need verification:
- TunnelConfig fields: `Version`, `Encap`, `Peer`, `Local`, `TunnelID`, `PeerTunnelID`, `HelloTimeout`
- SessionConfig fields: `SessionID`, `Pseudowire`

### Constant Names
Verify these constant names match the actual library:
- `l2tp.ProtocolVersion2` (or `l2tp.ProtocolVersionV2`?)
- `l2tp.EncapTypeUDP` (or `l2tp.EncapUDP`?)
- `l2tp.PseudowireTypePPP` (or `l2tp.PseudowirePPP`?)

## Testing

Once network access is available and dependencies are downloaded:
1. Run `go build ./internal/client` to check for compilation errors
2. Fix any undefined constant/type errors
3. Implement event handlers for proper tunnel/session management
4. Test with actual L2TP server
