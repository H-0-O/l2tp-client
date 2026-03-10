package client

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	kitlog "github.com/go-kit/kit/log"
	"github.com/katalix/go-l2tp/l2tp"
	"github.com/l2tww/l2tp-client/internal/config"
	"github.com/l2tww/l2tp-client/internal/ppp"
)

// l2tpLogger implements go-kit Logger and parses keyvals for tunnel/session IDs
// to update client status when the library logs events.
type l2tpLogger struct {
	client *Client
}

func (l *l2tpLogger) Log(keyvals ...interface{}) error {
	// Forward to standard log
	log.Println(keyvals...)
	// Parse keyvals (pairs of key, value) for IDs
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		keyLower := strings.ToLower(strings.TrimSpace(key))
		v := toUint32(keyvals[i+1])
		l.client.mu.Lock()
		switch {
		case keyLower == "tunnel_id" || keyLower == "tunnelid":
			if v != 0 {
				l.client.status.TunnelID = v
				log.Printf("Captured TunnelID from log: %d", v)
			}
		case keyLower == "session_id" || keyLower == "sessionid":
			if v != 0 {
				l.client.status.SessionID = v
				log.Printf("Captured SessionID from log: %d", v)
			}
		case keyLower == "peer_tunnel_id" || keyLower == "peer_tunnelid" || keyLower == "peer_tunnel":
			if v != 0 {
				l.client.status.RemoteTunnelID = v
			}
		case keyLower == "peer_session_id" || keyLower == "peer_sessionid" || keyLower == "peer_session":
			if v != 0 {
				l.client.status.RemoteSessionID = v
			}
		}
		l.client.mu.Unlock()
	}
	return nil
}

func toUint32(v interface{}) uint32 {
	switch x := v.(type) {
	case uint32:
		return x
	case uint:
		return uint32(x)
	case uint64:
		return uint32(x)
	case uint16:
		return uint32(x)
	case uint8:
		return uint32(x)
	case int:
		if x > 0 {
			return uint32(x)
		}
	case int32:
		if x > 0 {
			return uint32(x)
		}
	case int64:
		if x > 0 {
			return uint32(x)
		}
	case int16:
		if x > 0 {
			return uint32(x)
		}
	case string:
		n, _ := strconv.ParseUint(x, 10, 32)
		return uint32(n)
	default:
		// Reflection fallback for any numeric type (e.g. from go-kit)
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if n := rv.Int(); n > 0 {
				return uint32(n)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return uint32(rv.Uint())
		}
	}
	return 0
}

// eventHandler captures peer tunnel/session IDs from library events for PPPoL2TP.
type eventHandler struct {
	client *Client
}

func (h *eventHandler) HandleEvent(event interface{}) {
	switch e := event.(type) {
	case *l2tp.TunnelUpEvent:
		if e.Config != nil && e.Config.PeerTunnelID != 0 {
			h.client.mu.Lock()
			h.client.status.RemoteTunnelID = uint32(e.Config.PeerTunnelID)
			h.client.mu.Unlock()
			log.Printf("Captured PeerTunnelID from event: %d", e.Config.PeerTunnelID)
		}
	case *l2tp.SessionUpEvent:
		if e.SessionConfig != nil && e.SessionConfig.PeerSessionID != 0 {
			h.client.mu.Lock()
			h.client.status.RemoteSessionID = uint32(e.SessionConfig.PeerSessionID)
			h.client.mu.Unlock()
			log.Printf("Captured PeerSessionID from event: %d", e.SessionConfig.PeerSessionID)
		}
	}
}

// PPPoL2TP constants and structures
const (
	PX_PROTO_OL2TP = 0x00000002
	AF_PPPOX       = 24
	PF_PPPOX       = AF_PPPOX

	// PPP ioctl constants (not defined in standard syscall package)
	PPPIOCGCHAN   = 0x40047437
	PPPIOCATTCHAN = 0x40047438
	PPPIOCNEWUNIT = 0xC004743E
	PPPIOCCONNECT = 0x4004743A
)

// sockaddr_pppol2tp represents the PPPoL2TP socket address for connect(2).
// Layout must match kernel: struct sockaddr_pppol2tp in if_pppox.h (sa_family, sa_protocol as unsigned int, then pppol2tp_addr).
// See linux/include/uapi/linux/if_pppox.h and if_pppol2tp.h.
type sockaddr_pppol2tp struct {
	sa_family   uint16   // 2 bytes
	sa_protocol uint32   // 4 bytes (kernel: unsigned int), must not be uint16 or layout is wrong
	pid         int32    // 0 => current process owns the fd
	fd          int32
	addr        [16]byte // sockaddr_in: family(2), port(2), ip(4), zero(8)
	s_tunnel    uint16
	s_session   uint16
	d_tunnel    uint16
	d_session   uint16
}

// Client represents an L2TP client connection
type Client struct {
	cfg        *config.Config
	l2tpCtx    *l2tp.Context
	tunnel     l2tp.Tunnel
	session    l2tp.Session
	pppManager *ppp.PPPManager
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	connected  bool
	status     Status
	tunnelCfg  *l2tp.TunnelConfig
	sessionCfg *l2tp.SessionConfig
	tunnelSock  int        // Tunnel socket FD for PPP connection (same as tunnelFile.Fd() when set)
	tunnelFile  *os.File   // Keeps tunnel socket alive for library and PPPoL2TP; closed on disconnect
	kernelL2TP  *kernelL2TP // Kernel L2TP tunnel/session for PPPoL2TP; nil if not used or failed
}

// Status represents the connection status
type Status struct {
	Connected   bool
	TunnelID    uint32
	SessionID   uint32
	RemoteTunnelID  uint32
	RemoteSessionID uint32
	Interface   string
	LastError   error
	ConnectedAt time.Time
}

// NewClient creates a new L2TP client
func NewClient(cfg *config.Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		status:  Status{},
		tunnelSock: -1,
	}

	// Create go-kit logger that parses tunnel/session IDs from library log keyvals
	logger := &l2tpLogger{client: client}
	// Wrap so NewContext receives a kitlog.Logger
	var kitLogger kitlog.Logger = logger

	// Create L2TP context with logger
	l2tpCtx, err := l2tp.NewContext(l2tp.LinuxNetlinkDataPlane, kitLogger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create L2TP context: %w", err)
	}
	client.l2tpCtx = l2tpCtx
	client.l2tpCtx.RegisterEventHandler(&eventHandler{client: client})

	// Create PPP manager
	pppCfg := &ppp.PPPConfig{
		Username:   cfg.Username,
		Password:   cfg.Password,
		AuthMethod: cfg.AuthMethod,
		Interface:  cfg.Interface,
		IPv4:       cfg.IPv4,
		IPv6:       cfg.IPv6,
	}
	client.pppManager = ppp.NewPPPManager(pppCfg)

	return client, nil
}

// Connect establishes an L2TP tunnel and session.
// Does not hold c.mu for the whole call so that Disconnect/Close can run on signal.
func (c *Client) Connect() error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	c.mu.Unlock()

	serverAddr := c.cfg.GetServerAddress()
	log.Printf("Connecting to L2TP server at %s", serverAddr)

	// Create UDP socket for the tunnel; keep the file so the FD stays valid for library and PPPoL2TP
	tunnelSock, tunnelFile, err := c.createTunnelSocket(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to create tunnel socket: %w", err)
	}

	// Create tunnel configuration for L2TPv2 dynamic tunnel (client mode).
	// Pass our socket FD when using local go-l2tp with Fd support (see go.mod replace).
	tunnelCfg := &l2tp.TunnelConfig{
		Version:      l2tp.ProtocolVersion2,
		Encap:        l2tp.EncapTypeUDP,
		Peer:         serverAddr,
		HelloTimeout: time.Duration(c.cfg.HelloTimeout) * time.Second,
		Fd:           tunnelSock,
	}
	fdSet := tunnelSock >= 0
	if fdSet {
		log.Printf("Using tunnel socket FD: %d (PPP-over-L2TP)", tunnelSock)
	} else {
		tunnelFile.Close()
		tunnelFile = nil
		tunnelSock = -1
	}

	// Create dynamic tunnel (L2TPv2 client mode)
	tunnel, err := c.l2tpCtx.NewDynamicTunnel("l2tp-client-tunnel", tunnelCfg)
	if err != nil {
		if tunnelFile != nil {
			tunnelFile.Close()
		}
		return fmt.Errorf("failed to create tunnel: %w", err)
	}
	c.mu.Lock()
	c.tunnel = tunnel
	c.tunnelCfg = tunnelCfg
	c.tunnelSock = tunnelSock
	c.tunnelFile = tunnelFile
	c.mu.Unlock()

	// Create session configuration
	sessionCfg := &l2tp.SessionConfig{
		Pseudowire: l2tp.PseudowireTypePPP,
	}

	// Create session
	session, err := tunnel.NewSession("l2tp-client-session", sessionCfg)
	if err != nil {
		tunnel.Close()
		if tunnelFile != nil {
			tunnelFile.Close()
		}
		return fmt.Errorf("failed to create session: %w", err)
	}
	c.mu.Lock()
	c.session = session
	c.sessionCfg = sessionCfg
	c.mu.Unlock()

	// Wait for tunnel and session establishment: poll for non-zero IDs (from logger or config)
	const waitTimeout = 20 * time.Second
	const pollInterval = 200 * time.Millisecond
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-c.ctx.Done():
			tunnel.Close()
			if tunnelFile != nil {
				tunnelFile.Close()
			}
			c.mu.Lock()
			c.tunnel, c.session, c.tunnelFile = nil, nil, nil
			c.tunnelSock = -1
			c.mu.Unlock()
			return c.ctx.Err()
		default:
		}
		c.readIDsFromConfig()
		c.mu.Lock()
		tid, sid := c.status.TunnelID, c.status.SessionID
		peerTid, peerSid := c.status.RemoteTunnelID, c.status.RemoteSessionID
		c.mu.Unlock()
		if tid != 0 && sid != 0 && peerTid != 0 && peerSid != 0 {
			break
		}
		select {
		case <-c.ctx.Done():
			tunnel.Close()
			if tunnelFile != nil {
				tunnelFile.Close()
			}
			c.mu.Lock()
			c.tunnel, c.session, c.tunnelFile = nil, nil, nil
			c.tunnelSock = -1
			c.mu.Unlock()
			return c.ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	c.mu.Lock()
	tunnelID := c.status.TunnelID
	sessionID := c.status.SessionID
	peerTunnelID := c.status.RemoteTunnelID
	peerSessionID := c.status.RemoteSessionID
	c.mu.Unlock()

	if tunnelID == 0 || sessionID == 0 {
		tunnel.Close()
		if tunnelFile != nil {
			tunnelFile.Close()
		}
		c.mu.Lock()
		c.tunnel, c.session, c.tunnelFile = nil, nil, nil
		c.tunnelSock = -1
		c.mu.Unlock()
		return fmt.Errorf("tunnel/session establishment timeout: TunnelID=%d, SessionID=%d (IDs not available from library)", tunnelID, sessionID)
	}
	if peerTunnelID == 0 || peerSessionID == 0 {
		log.Printf("Warning: peer IDs not yet available (PeerTunnelID=%d, PeerSessionID=%d); PPP socket may fail", peerTunnelID, peerSessionID)
	}

	// Update status and mark connected (so Disconnect/Close can tear down)
	c.mu.Lock()
	c.status.Connected = true
	c.status.ConnectedAt = time.Now()
	c.status.Interface = c.cfg.Interface
	c.connected = true
	c.mu.Unlock()

	log.Printf("L2TP tunnel established: TunnelID=%d, SessionID=%d, PeerTunnelID=%d, PeerSessionID=%d", tunnelID, sessionID, peerTunnelID, peerSessionID)

	// With LinuxNetlinkDataPlane, go-l2tp already registers the tunnel and session with the kernel
	// when the control plane establishes (dp.NewTunnel / dp.NewSession). We must not register again
	// or the kernel returns "file exists". Go straight to createPPPSocket() so PPPoL2TP connect()
	// uses the kernel tunnel/session that go-l2tp created.

	// Decide whether to create a real PPP interface or stay with the pty fallback based on config.
	if !c.cfg.CreatePPPInterface {
		log.Printf("Config: CreatePPPInterface=false; skipping PPPoL2TP device and using pty fallback")
		c.pppManager.SetPtyPath("echo 'PPP interface creation disabled by config'")
	} else if c.tunnelSock != -1 {
		// Create PPP socket only when we have non-zero IDs and valid tunnel FD
		pppDevice, err := c.createPPPSocket()
		if err != nil {
			log.Printf("Warning: failed to create PPP socket: %v", err)
			c.pppManager.SetPtyPath("echo 'PPP socket creation failed - using placeholder'")
		} else {
			c.pppManager.SetPtyPath(pppDevice)
		}
	} else {
		log.Printf("Tunnel socket FD not available (library does not expose it); cannot create PPP socket, using pty fallback")
		c.pppManager.SetPtyPath("echo 'L2TP PPP placeholder - tunnel FD not available'")
	}

	// Start PPP manager
	if err := c.pppManager.Start(); err != nil {
		log.Printf("Warning: failed to start pppd: %v", err)
		// Continue anyway - tunnel is established
	}

	// Start monitoring goroutine
	c.wg.Add(1)
	go c.monitor()

	return nil
}

// Disconnect tears down the L2TP tunnel and session.
// Does teardown outside the lock and with a timeout so signal handler can return and process can exit.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	pppMgr := c.pppManager
	session := c.session
	tunnel := c.tunnel
	tunnelFile := c.tunnelFile
	kl2tp := c.kernelL2TP
	tid, sid := c.status.TunnelID, c.status.SessionID
	c.connected = false
	c.status.Connected = false
	c.session = nil
	c.tunnel = nil
	c.tunnelFile = nil
	c.tunnelSock = -1
	c.kernelL2TP = nil
	c.mu.Unlock()

	log.Printf("Disconnecting from L2TP server")

	// Run teardown with timeout so we never block forever (pppd or library Close can hang).
	// Recover any panic from go-l2tp (e.g. "send on closed channel" during shutdown).
	// Close tunnelFile only after tunnel/session close so the library can still use the FD to send CDN etc.
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Disconnect teardown: %v", r)
			}
			if tunnelFile != nil {
				_ = tunnelFile.Close()
			}
			close(done)
		}()
		if pppMgr != nil {
			_ = pppMgr.Stop()
		}
		if kl2tp != nil && tid != 0 {
			_ = kl2tp.UnregisterSession(tid, sid)
			_ = kl2tp.UnregisterTunnel(tid)
			_ = kl2tp.Close()
		}
		if session != nil {
			session.Close()
		}
		if tunnel != nil {
			tunnel.Close()
		}
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		log.Printf("Disconnect timeout (6s), exiting anyway")
	}

	log.Printf("Disconnected from L2TP server")
	return nil
}

// GetStatus returns the current connection status
func (c *Client) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// readIDsFromConfig updates c.status with tunnel/session IDs from config structs (if set by library).
func (c *Client) readIDsFromConfig() {
	var tid, ptid, sid, psid uint32
	if c.tunnelCfg != nil {
		v := reflect.ValueOf(c.tunnelCfg).Elem()
		for _, name := range []string{"TunnelID", "TunnelId"} {
			f := v.FieldByName(name)
			if f.IsValid() && f.CanInterface() {
				if u := toUint32(f.Interface()); u != 0 {
					tid = u
					break
				}
			}
		}
		for _, name := range []string{"PeerTunnelID", "PeerTunnelId"} {
			f := v.FieldByName(name)
			if f.IsValid() && f.CanInterface() {
				if u := toUint32(f.Interface()); u != 0 {
					ptid = u
					break
				}
			}
		}
	}
	if c.sessionCfg != nil {
		v := reflect.ValueOf(c.sessionCfg).Elem()
		for _, name := range []string{"SessionID", "SessionId"} {
			f := v.FieldByName(name)
			if f.IsValid() && f.CanInterface() {
				if u := toUint32(f.Interface()); u != 0 {
					sid = u
					break
				}
			}
		}
		for _, name := range []string{"PeerSessionID", "PeerSessionId"} {
			f := v.FieldByName(name)
			if f.IsValid() && f.CanInterface() {
				if u := toUint32(f.Interface()); u != 0 {
					psid = u
					break
				}
			}
		}
	}
	// Option C: try to get IDs from tunnel/session (concrete types behind interfaces)
	if c.tunnel != nil {
		v := reflect.ValueOf(c.tunnel)
		for v.IsValid() && (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) {
			v = v.Elem()
		}
		for _, name := range []string{"TunnelID", "TunnelId", "ID"} {
			m := v.MethodByName(name)
			if !m.IsValid() {
				continue
			}
			out := m.Call(nil)
			if len(out) > 0 && out[0].CanInterface() {
				if u := toUint32(out[0].Interface()); u != 0 {
					tid = u
					break
				}
			}
		}
	}
	if c.session != nil {
		v := reflect.ValueOf(c.session)
		for v.IsValid() && (v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface) {
			v = v.Elem()
		}
		for _, name := range []string{"SessionID", "SessionId", "ID"} {
			m := v.MethodByName(name)
			if !m.IsValid() {
				continue
			}
			out := m.Call(nil)
			if len(out) > 0 && out[0].CanInterface() {
				if u := toUint32(out[0].Interface()); u != 0 {
					sid = u
					break
				}
			}
		}
	}

	if tid != 0 || ptid != 0 || sid != 0 || psid != 0 {
		c.mu.Lock()
		if tid != 0 {
			c.status.TunnelID = tid
		}
		if ptid != 0 {
			c.status.RemoteTunnelID = ptid
		}
		if sid != 0 {
			c.status.SessionID = sid
		}
		if psid != 0 {
			c.status.RemoteSessionID = psid
		}
		c.mu.Unlock()
	}
}

// createTunnelSocket creates a UDP socket for the L2TP tunnel.
// Returns (fd, file, nil). The caller must keep file open for the FD to stay valid for the library and PPPoL2TP;
// when not passing the FD to the library, the caller should close the file and use fd = -1.
func (c *Client) createTunnelSocket(serverAddr string) (fd int, file *os.File, err error) {
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return -1, nil, fmt.Errorf("failed to resolve server address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return -1, nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}
	f, err := conn.File()
	if err != nil {
		conn.Close()
		return -1, nil, fmt.Errorf("failed to get socket file: %w", err)
	}
	conn.Close() // file is a dup; socket stays open via f
	return int(f.Fd()), f, nil
}

// createPPPSocket creates a PPPoL2TP socket and connects it to the L2TP session.
// Returns the PPP device name if successful. Requires valid c.tunnelSock (same FD as L2TP tunnel).
func (c *Client) createPPPSocket() (string, error) {
	c.mu.RLock()
	tunnelID := c.status.TunnelID
	sessionID := c.status.SessionID
	peerTunnelID := c.status.RemoteTunnelID
	peerSessionID := c.status.RemoteSessionID
	c.mu.RUnlock()

	if tunnelID == 0 || sessionID == 0 {
		return "", fmt.Errorf("tunnel or session ID not available")
	}
	if c.tunnelSock == -1 {
		return "", fmt.Errorf("tunnel socket FD not available (library does not expose it); cannot create PPP socket")
	}

	// Create PPPoL2TP socket
	pppSock, err := syscall.Socket(AF_PPPOX, syscall.SOCK_DGRAM, PX_PROTO_OL2TP)
	if err != nil {
		return "", fmt.Errorf("failed to create PPPoL2TP socket: %w", err)
	}
	defer syscall.Close(pppSock)

	// Resolve peer address for kernel (server IP and port)
	serverAddr := c.cfg.GetServerAddress()
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return "", fmt.Errorf("failed to resolve server address for PPP: %w", err)
	}
	ip := udpAddr.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("server address is not IPv4: %s", serverAddr)
	}
	// Fill sockaddr_in in sax.addr: sin_family(2), sin_port(2 net order), sin_addr(4), sin_zero(8)
	var addr [16]byte
	addr[0] = byte(syscall.AF_INET)
	addr[1] = 0
	port := uint16(udpAddr.Port)
	addr[2] = byte(port >> 8)
	addr[3] = byte(port)
	copy(addr[4:8], ip)
	// addr[8:16] already zero

	// Prepare sockaddr_pppol2tp structure (pid=0: current process owns the FD)
	sax := sockaddr_pppol2tp{
		sa_family:   AF_PPPOX,
		sa_protocol: PX_PROTO_OL2TP,
		pid:         0,
		fd:          int32(c.tunnelSock),
		addr:        addr,
		s_tunnel:    uint16(tunnelID),
		s_session:   uint16(sessionID),
		d_tunnel:    uint16(peerTunnelID),
		d_session:   uint16(peerSessionID),
	}

	// Connect the PPP socket to the L2TP session
	_, _, errno := syscall.Syscall(syscall.SYS_CONNECT, uintptr(pppSock), uintptr(unsafe.Pointer(&sax)), unsafe.Sizeof(sax))
	if errno != 0 {
		return "", fmt.Errorf("failed to connect PPP socket: %v", errno)
	}

	// Get channel number for the PPP socket
	var chindx int
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(pppSock), PPPIOCGCHAN, uintptr(unsafe.Pointer(&chindx)))
	if errno != 0 {
		return "", fmt.Errorf("failed to get PPP channel: %v", errno)
	}

	// Open PPP device and attach channel
	pppFd, err := syscall.Open("/dev/ppp", syscall.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/ppp: %w", err)
	}

	// Attach channel to PPP device
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(pppFd), PPPIOCATTCHAN, uintptr(chindx))
	if errno != 0 {
		syscall.Close(pppFd)
		return "", fmt.Errorf("failed to attach PPP channel: %v", errno)
	}

	// Create PPP interface unit
	var ifunit int = -1
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(pppFd), PPPIOCNEWUNIT, uintptr(unsafe.Pointer(&ifunit)))
	if errno != 0 {
		syscall.Close(pppFd)
		return "", fmt.Errorf("failed to create PPP unit: %v", errno)
	}

	// Connect channel to unit
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, uintptr(pppFd), PPPIOCCONNECT, uintptr(ifunit))
	if errno != 0 {
		syscall.Close(pppFd)
		return "", fmt.Errorf("failed to connect PPP channel: %v", errno)
	}

	deviceName := fmt.Sprintf("ppp%d", ifunit)
	log.Printf("PPP socket created and connected - device %s", deviceName)
	return deviceName, nil
}

// Close closes the client and cleans up resources.
// Call this on shutdown/signal so the wait loop in Connect() can exit and resources are freed.
func (c *Client) Close() error {
	c.cancel()

	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()

	if connected {
		if err := c.Disconnect(); err != nil {
			return err
		}
	} else {
		// Clean up tunnel/session if we had started but not yet marked connected (e.g. cancelled in wait loop)
		c.mu.Lock()
		tunnel := c.tunnel
		session := c.session
		tunnelFile := c.tunnelFile
		c.tunnel = nil
		c.session = nil
		c.tunnelFile = nil
		c.tunnelSock = -1
		c.mu.Unlock()
		if tunnelFile != nil {
			_ = tunnelFile.Close()
		}
		// Don't hold lock during Close() - library can block or panic (e.g. "send on closed channel")
		if tunnel != nil {
			done := make(chan struct{})
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Tunnel close: %v", r)
					}
					close(done)
				}()
				if session != nil {
					session.Close()
				}
				tunnel.Close()
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				log.Printf("Tunnel close timeout, exiting")
			}
		}
	}

	c.wg.Wait()

	return nil
}

// monitor monitors the connection and handles reconnection if enabled
func (c *Client) monitor() {
	defer c.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()

			if !connected && c.cfg.AutoReconnect {
				log.Printf("Connection lost, attempting to reconnect...")
				c.mu.Lock()
				c.connected = false // Reset state
				c.mu.Unlock()

				time.Sleep(time.Duration(c.cfg.ReconnectDelay) * time.Second)

				if err := c.Connect(); err != nil {
					log.Printf("Reconnection failed: %v", err)
					c.mu.Lock()
					c.status.LastError = err
					c.mu.Unlock()
				} else {
					log.Printf("Reconnected successfully")
				}
			}
		}
	}
}
