# Quick Start Guide

## Prerequisites

1. **Install kernel modules** (if not already loaded):
   ```bash
   sudo modprobe l2tp_core l2tp_netlink l2tp_eth l2tp_ip l2tp_ip6
   ```

2. **Install pppd** (if not already installed):
   ```bash
   # Ubuntu/Debian
   sudo apt-get install ppp
   
   # Fedora/RHEL
   sudo dnf install ppp
   ```

3. **Install Go dependencies**:
   ```bash
   go mod download
   ```

## Building

```bash
make build
```

Or manually:
```bash
go build -o l2tp-client ./cmd/l2tp-client
```

## Basic Usage

### Connect to an L2TP server

```bash
sudo ./l2tp-client connect \
  --server vpn.example.com \
  --user myusername \
  --password mypassword
```

### Using environment variables

```bash
export L2TP_SERVER=vpn.example.com
export L2TP_USERNAME=myusername
export L2TP_PASSWORD=mypassword

sudo ./l2tp-client connect
```

### Using a configuration file

1. Copy the example config:
   ```bash
   cp example-config.toml my-config.toml
   ```

2. Edit `my-config.toml` with your settings

3. Connect:
   ```bash
   sudo ./l2tp-client connect --config my-config.toml
   ```

## Checking Status

```bash
sudo ./l2tp-client status
```

## Disconnecting

Press `Ctrl+C` while connected, or:

```bash
sudo ./l2tp-client disconnect
```

## Troubleshooting

### "Permission denied" errors

The L2TP client requires root privileges to interact with the Linux kernel L2TP subsystem. Always run with `sudo`.

### "Module not found" errors

Ensure the L2TP kernel modules are loaded:
```bash
lsmod | grep l2tp
```

If no modules are listed, load them:
```bash
sudo modprobe l2tp_core l2tp_netlink l2tp_eth l2tp_ip l2tp_ip6
```

### "pppd not found" errors

Install the ppp package for your distribution.

### Connection fails

1. Verify the server address and port are correct
2. Check firewall settings (L2TP uses UDP port 1701 by default)
3. Verify your credentials are correct
4. Check server logs if you have access
