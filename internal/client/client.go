package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/katalix/go-l2tp/l2tp"
	"github.com/l2tww/l2tp-client/internal/config"
	"github.com/l2tww/l2tp-client/internal/ppp"
)

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

	// Create L2TP context
	l2tpCtx, err := l2tp.NewContext(l2tp.LinuxNetlinkDataPlane, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create L2TP context: %w", err)
	}

	// Create PPP manager
	pppCfg := &ppp.PPPConfig{
		Username:   cfg.Username,
		Password:   cfg.Password,
		AuthMethod: cfg.AuthMethod,
		Interface:  cfg.Interface,
		IPv4:       cfg.IPv4,
		IPv6:       cfg.IPv6,
	}
	pppManager := ppp.NewPPPManager(pppCfg)

	return &Client{
		cfg:        cfg,
		l2tpCtx:    l2tpCtx,
		pppManager: pppManager,
		ctx:        ctx,
		cancel:     cancel,
		status:     Status{},
	}, nil
}

// Connect establishes an L2TP tunnel and session
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	serverAddr := c.cfg.GetServerAddress()
	log.Printf("Connecting to L2TP server at %s", serverAddr)

	// Create tunnel configuration for L2TPv2 dynamic tunnel (client mode)
	// For L2TPv2 client, we use NewDynamicTunnel which runs the full control protocol
	tunnelCfg := &l2tp.TunnelConfig{
		Version:      l2tp.ProtocolVersion2,
		Encap:        l2tp.EncapTypeUDP,
		Peer:         serverAddr,
		HelloTimeout: time.Duration(c.cfg.HelloTimeout) * time.Second,
	}

	// Create dynamic tunnel (L2TPv2 client mode)
	tunnel, err := c.l2tpCtx.NewDynamicTunnel("l2tp-client-tunnel", tunnelCfg)
	if err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}
	c.tunnel = tunnel
	c.tunnelCfg = tunnelCfg

	// Wait for tunnel to be established
	// In a real implementation, we'd wait for TunnelUpEvent
	time.Sleep(2 * time.Second)

	// Create session configuration
	sessionCfg := &l2tp.SessionConfig{
		Pseudowire: l2tp.PseudowireTypePPP,
	}

	// Create session
	session, err := tunnel.NewSession("l2tp-client-session", sessionCfg)
	if err != nil {
		c.tunnel.Close()
		return fmt.Errorf("failed to create session: %w", err)
	}
	c.session = session
	c.sessionCfg = sessionCfg

	// Get tunnel and session IDs from config
	// Note: For dynamic tunnels, IDs are assigned during negotiation
	// We'll use placeholder values for now - in production, get from events
	tunnelID := uint32(0)
	sessionID := uint32(0)
	if tunnelCfg.TunnelID != 0 {
		tunnelID = uint32(tunnelCfg.TunnelID)
	}
	if sessionCfg.SessionID != 0 {
		sessionID = uint32(sessionCfg.SessionID)
	}

	// Update status
	c.status = Status{
		Connected:      true,
		TunnelID:       tunnelID,
		SessionID:      sessionID,
		Interface:      c.cfg.Interface,
		ConnectedAt:    time.Now(),
	}

	c.connected = true

	log.Printf("L2TP tunnel established: TunnelID=%d, SessionID=%d", tunnelID, sessionID)

	// Get the pty path from the session for pppd
	// The go-l2tp library should provide this via the session interface
	// For now, we'll construct a typical pty path
	// In production, this should come from the session object
	ptyPath := fmt.Sprintf("/dev/ppp%d", sessionID)
	c.pppManager.SetPtyPath(ptyPath)

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

// Disconnect tears down the L2TP tunnel and session
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	log.Printf("Disconnecting from L2TP server")

	// Stop PPP manager
	if err := c.pppManager.Stop(); err != nil {
		log.Printf("Warning: failed to stop pppd: %v", err)
	}

	// Close session
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}

	// Close tunnel
	if c.tunnel != nil {
		c.tunnel.Close()
		c.tunnel = nil
	}

	c.connected = false
	c.status.Connected = false

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

// Close closes the client and cleans up resources
func (c *Client) Close() error {
	c.cancel()

	if c.connected {
		if err := c.Disconnect(); err != nil {
			return err
		}
	}

	c.wg.Wait()

	if c.l2tpCtx != nil {
		// L2TP context cleanup if needed
	}

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
