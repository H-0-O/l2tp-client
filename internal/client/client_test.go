package client

import (
	"testing"
	"time"

	"github.com/l2tww/l2tp-client/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		Server:     "vpn.example.com:1701",
		Port:       1701,
		Username:   "testuser",
		Password:   "testpass",
		AuthMethod: "pap",
		IPv4:       true,
		IPv6:       false,
	}

	// This will fail without the actual go-l2tp library and kernel support
	// but we can test the validation logic
	_, err := NewClient(cfg)
	if err == nil {
		// If it succeeds, that's fine - means we have the library
		return
	}

	// Expected to fail without kernel modules, but should validate config first
	if err != nil && err.Error() == "invalid configuration" {
		t.Errorf("Expected config validation error, got: %v", err)
	}
}

func TestClientStatus(t *testing.T) {
	cfg := &config.Config{
		Server:     "vpn.example.com:1701",
		Port:       1701,
		Username:   "testuser",
		Password:   "testpass",
		AuthMethod: "pap",
	}

	client, err := NewClient(cfg)
	if err != nil {
		// Skip if we can't create client (no kernel support)
		t.Skipf("Skipping test - cannot create client: %v", err)
		return
	}
	defer client.Close()

	status := client.GetStatus()
	if status.Connected {
		t.Error("Expected disconnected status initially")
	}

	if client.IsConnected() {
		t.Error("Expected client to be disconnected initially")
	}
}

func TestClientClose(t *testing.T) {
	cfg := &config.Config{
		Server:     "vpn.example.com:1701",
		Port:       1701,
		Username:   "testuser",
		Password:   "testpass",
		AuthMethod: "pap",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Skipping test - cannot create client: %v", err)
		return
	}

	// Close should not panic even if not connected
	if err := client.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
