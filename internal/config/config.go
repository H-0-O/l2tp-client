package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the L2TP client configuration
type Config struct {
	Server      string
	Port        int
	Username    string
	Password    string
	AuthMethod  string
	Interface   string
	IPv4        bool
	IPv6        bool
	AutoReconnect bool
	ReconnectDelay int
	HelloTimeout   int
	ConfigFile     string
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Port:          1701,
		AuthMethod:    "pap",
		IPv4:          true,
		IPv6:          false,
		AutoReconnect: false,
		ReconnectDelay: 5,
		HelloTimeout:   60,
	}
}

// LoadConfig loads configuration from command-line flags, environment variables, and config file
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Load from config file if specified
	if viper.GetString("config") != "" {
		viper.SetConfigFile(viper.GetString("config"))
		if err := viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Load from environment variables
	viper.SetEnvPrefix("L2TP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Override with command-line flags
	if viper.IsSet("server") {
		cfg.Server = viper.GetString("server")
	}
	if viper.IsSet("port") {
		cfg.Port = viper.GetInt("port")
	}
	if viper.IsSet("username") {
		cfg.Username = viper.GetString("username")
	}
	if viper.IsSet("password") {
		cfg.Password = viper.GetString("password")
	} else if cfg.Username != "" {
		// Try to get password from environment
		if pwd := os.Getenv("L2TP_PASSWORD"); pwd != "" {
			cfg.Password = pwd
		}
	}
	if viper.IsSet("auth") {
		cfg.AuthMethod = strings.ToLower(viper.GetString("auth"))
	}
	if viper.IsSet("interface") {
		cfg.Interface = viper.GetString("interface")
	}
	if viper.IsSet("ipv4") {
		cfg.IPv4 = viper.GetBool("ipv4")
	}
	if viper.IsSet("ipv6") {
		cfg.IPv6 = viper.GetBool("ipv6")
	}
	if viper.IsSet("auto-reconnect") {
		cfg.AutoReconnect = viper.GetBool("auto-reconnect")
	}
	if viper.IsSet("reconnect-delay") {
		cfg.ReconnectDelay = viper.GetInt("reconnect-delay")
	}
	if viper.IsSet("hello-timeout") {
		cfg.HelloTimeout = viper.GetInt("hello-timeout")
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server == "" {
		return fmt.Errorf("server address is required")
	}

	// Validate server address format
	host, port, err := net.SplitHostPort(c.Server)
	if err != nil {
		// Try to parse as hostname/IP only
		if net.ParseIP(c.Server) == nil {
			// Check if it's a valid hostname
			if _, err := net.LookupHost(c.Server); err != nil {
				return fmt.Errorf("invalid server address: %w", err)
			}
		}
	} else {
		if host == "" {
			return fmt.Errorf("server address must include hostname or IP")
		}
		if port != "" && port != fmt.Sprintf("%d", c.Port) {
			// Port mismatch, but we'll use the one from SplitHostPort
		}
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	if c.Username == "" {
		return fmt.Errorf("username is required")
	}

	if c.Password == "" {
		return fmt.Errorf("password is required (use --password flag or L2TP_PASSWORD env var)")
	}

	validAuthMethods := map[string]bool{
		"pap":        true,
		"chap":       true,
		"mschap":     true,
		"mschapv2":   true,
		"eap":        false, // Not implemented yet
	}
	if !validAuthMethods[c.AuthMethod] {
		return fmt.Errorf("invalid auth method: %s (supported: pap, chap, mschap, mschapv2)", c.AuthMethod)
	}

	if c.ReconnectDelay < 1 {
		return fmt.Errorf("reconnect delay must be at least 1 second")
	}

	if c.HelloTimeout < 0 {
		return fmt.Errorf("hello timeout must be non-negative")
	}

	return nil
}

// GetServerAddress returns the full server address with port
func (c *Config) GetServerAddress() string {
	if strings.Contains(c.Server, ":") {
		return c.Server
	}
	return fmt.Sprintf("%s:%d", c.Server, c.Port)
}
