package config

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != 1701 {
		t.Errorf("Expected default port 1701, got %d", cfg.Port)
	}

	if cfg.AuthMethod != "pap" {
		t.Errorf("Expected default auth method 'pap', got '%s'", cfg.AuthMethod)
	}

	if !cfg.IPv4 {
		t.Error("Expected IPv4 to be enabled by default")
	}

	if cfg.IPv6 {
		t.Error("Expected IPv6 to be disabled by default")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Server:   "vpn.example.com:1701",
				Port:     1701,
				Username: "testuser",
				Password: "testpass",
				AuthMethod: "pap",
			},
			wantErr: false,
		},
		{
			name: "missing server",
			config: &Config{
				Port:     1701,
				Username: "testuser",
				Password: "testpass",
			},
			wantErr: true,
		},
		{
			name: "missing username",
			config: &Config{
				Server:   "vpn.example.com",
				Port:     1701,
				Password: "testpass",
			},
			wantErr: true,
		},
		{
			name: "missing password",
			config: &Config{
				Server:   "vpn.example.com",
				Port:     1701,
				Username: "testuser",
			},
			wantErr: true,
		},
		{
			name: "invalid auth method",
			config: &Config{
				Server:    "vpn.example.com",
				Port:      1701,
				Username:  "testuser",
				Password:  "testpass",
				AuthMethod: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: &Config{
				Server:   "vpn.example.com",
				Port:     70000,
				Username: "testuser",
				Password: "testpass",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetServerAddress(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "server with port",
			config: &Config{
				Server: "vpn.example.com:1701",
				Port:   1701,
			},
			expected: "vpn.example.com:1701",
		},
		{
			name: "server without port",
			config: &Config{
				Server: "vpn.example.com",
				Port:   1701,
			},
			expected: "vpn.example.com:1701",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetServerAddress()
			if result != tt.expected {
				t.Errorf("GetServerAddress() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("L2TP_SERVER", "test.example.com")
	os.Setenv("L2TP_PORT", "1702")
	os.Setenv("L2TP_USERNAME", "envuser")
	os.Setenv("L2TP_PASSWORD", "envpass")
	defer func() {
		os.Unsetenv("L2TP_SERVER")
		os.Unsetenv("L2TP_PORT")
		os.Unsetenv("L2TP_USERNAME")
		os.Unsetenv("L2TP_PASSWORD")
	}()

	viper.Reset()
	viper.SetEnvPrefix("L2TP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Server != "test.example.com" {
		t.Errorf("Expected server 'test.example.com', got '%s'", cfg.Server)
	}

	if cfg.Username != "envuser" {
		t.Errorf("Expected username 'envuser', got '%s'", cfg.Username)
	}
}
