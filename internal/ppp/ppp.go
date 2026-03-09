package ppp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// PPPConfig holds configuration for pppd
type PPPConfig struct {
	Username    string
	Password    string
	AuthMethod  string
	Interface   string
	IPv4        bool
	IPv6        bool
	PtyScript   string
	OptionsFile string
}

// PPPManager manages pppd process
type PPPManager struct {
	config      *PPPConfig
	cmd         *exec.Cmd
	pidFile     string
	optionsFile string
	ptyPath     string
}

// NewPPPManager creates a new PPP manager
func NewPPPManager(config *PPPConfig) *PPPManager {
	return &PPPManager{
		config: config,
	}
}

// SetPtyPath sets the pty path for the L2TP session
func (p *PPPManager) SetPtyPath(ptyPath string) {
	p.ptyPath = ptyPath
}

// Start starts the pppd process
func (p *PPPManager) Start() error {
	if p.ptyPath == "" {
		return fmt.Errorf("pty path not set - L2TP session must be established first")
	}

	// Create options file for pppd
	optionsFile, err := p.createOptionsFile()
	if err != nil {
		return fmt.Errorf("failed to create pppd options file: %w", err)
	}
	p.optionsFile = optionsFile

	// Use the pty path from the L2TP session
	ptyScript := p.ptyPath
	if p.config.PtyScript != "" {
		ptyScript = p.config.PtyScript
	}

	// Build pppd command
	args := []string{
		"nodetach",
		"noauth",
		"lock",
		"user", p.config.Username,
		"password", p.config.Password,
	}

	// Add authentication method
	switch strings.ToLower(p.config.AuthMethod) {
	case "pap":
		args = append(args, "require-pap")
	case "chap":
		args = append(args, "require-chap")
	case "mschap", "mschapv2":
		args = append(args, "require-mschap-v2")
	}

	// Add interface name if specified
	if p.config.Interface != "" {
		args = append(args, "linkname", p.config.Interface)
	}

	// Add IP configuration
	if p.config.IPv4 {
		args = append(args, "ipcp-accept-local", "ipcp-accept-remote")
	}
	if p.config.IPv6 {
		args = append(args, "+ipv6", "ipv6cp-use-ipaddr")
	}

	// Add pty - pppd will connect to the L2TP tunnel via this pty
	args = append(args, ptyScript)

	// Add options file if specified
	if p.config.OptionsFile != "" {
		args = append(args, "file", p.config.OptionsFile)
	}

	p.cmd = exec.Command("pppd", args...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pppd: %w", err)
	}

	return nil
}

// Stop stops the pppd process
func (p *PPPManager) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM to pppd
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If process already exited, that's fine
		if err.Error() != "os: process already finished" {
			return fmt.Errorf("failed to stop pppd: %w", err)
		}
	}

	// Wait for process to exit
	_ = p.cmd.Wait()

	// Clean up files
	p.cleanup()

	return nil
}

// IsRunning checks if pppd is running
func (p *PPPManager) IsRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}

	// Check if process is still running
	err := p.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// createOptionsFile creates a temporary options file for pppd
func (p *PPPManager) createOptionsFile() (string, error) {
	tmpDir := os.TempDir()
	optionsFile := filepath.Join(tmpDir, fmt.Sprintf("l2tp-pppd-%d.conf", os.Getpid()))

	content := []string{
		"# L2TP client pppd options",
		"lock",
		"noauth",
		"nodetach",
	}

	if p.config.IPv4 {
		content = append(content, "ipcp-accept-local", "ipcp-accept-remote")
	}
	if p.config.IPv6 {
		content = append(content, "+ipv6", "ipv6cp-use-ipaddr")
	}

	contentStr := strings.Join(content, "\n") + "\n"

	if err := os.WriteFile(optionsFile, []byte(contentStr), 0600); err != nil {
		return "", err
	}

	return optionsFile, nil
}

// createPtyScript creates a script that connects pppd to the L2TP tunnel
func (p *PPPManager) createPtyScript() (string, error) {
	tmpDir := os.TempDir()
	scriptFile := filepath.Join(tmpDir, fmt.Sprintf("l2tp-pty-%d.sh", os.Getpid()))

	// This script will be replaced by the actual L2TP tunnel connection
	// For now, we'll create a placeholder
	script := `#!/bin/sh
# L2TP pty script
# This will be replaced by the actual tunnel connection
exec cat
`

	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		return "", err
	}

	return scriptFile, nil
}

// cleanup removes temporary files
func (p *PPPManager) cleanup() {
	if p.optionsFile != "" {
		_ = os.Remove(p.optionsFile)
	}
	if p.pidFile != "" {
		_ = os.Remove(p.pidFile)
	}
}
