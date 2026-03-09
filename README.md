# L2TP Client

A user-friendly L2TP (Layer 2 Tunneling Protocol) command-line client for Linux systems, written in Go.

## Features

- **Simple CLI interface**: Easy-to-use command-line interface for connecting to L2TP servers
- **L2TPv2 support**: Full support for L2TPv2 protocol
- **Multiple authentication methods**: Supports PAP, CHAP, MS-CHAP, and MS-CHAPv2
- **IPv4/IPv6 support**: Configure IPv4 and/or IPv6 addressing
- **Auto-reconnection**: Optional automatic reconnection on connection failure
- **Status monitoring**: Check connection status and statistics
- **Configuration flexibility**: Support for command-line flags, environment variables, and config files

## Requirements

- Linux kernel with L2TP support (kernel modules: `l2tp_core`, `l2tp_netlink`, `l2tp_eth`, `l2tp_ip`, `l2tp_ip6`)
- `pppd` (Point-to-Point Protocol daemon) installed
- Go 1.21 or later (for building from source)
- Root privileges (required for L2TP kernel operations)

### Installing Kernel Modules

On Ubuntu/Debian:
```bash
sudo apt-get install linux-modules-extra-$(uname -r)
sudo modprobe l2tp_core l2tp_netlink l2tp_eth l2tp_ip l2tp_ip6
```

## Installation

### From Source

```bash
git clone https://github.com/l2tww/l2tp-client.git
cd l2tp-client
go mod download
go build -o l2tp-client ./cmd/l2tp-client
sudo cp l2tp-client /usr/local/bin/
```

## Usage

### Basic Connection

```bash
sudo l2tp-client connect --server vpn.example.com --user myusername --password mypassword
```

### Using Environment Variables

```bash
export L2TP_SERVER=vpn.example.com
export L2TP_USERNAME=myusername
export L2TP_PASSWORD=mypassword
sudo l2tp-client connect
```

### Command-Line Options

```
Flags:
  --server string          L2TP server address (hostname or IP)
  --port int               L2TP server port (default 1701)
  --user string            Username for authentication
  --password string        Password for authentication (or use L2TP_PASSWORD env var)
  --auth string            Authentication method: pap, chap, mschap, mschapv2 (default "pap")
  --interface string       Network interface name
  --ipv4                   Enable IPv4 (default true)
  --ipv6                   Enable IPv6
  --auto-reconnect         Automatically reconnect on failure
  --reconnect-delay int    Delay in seconds before reconnecting (default 5)
  --hello-timeout int      L2TP hello timeout in seconds (default 60)
  --config string          Config file path
```

### Commands

#### Connect

Establish an L2TP tunnel and session:

```bash
sudo l2tp-client connect --server vpn.example.com --user myuser --password mypass
```

The connection will remain active until interrupted (Ctrl+C) or disconnected.

#### Status

Check the current connection status:

```bash
sudo l2tp-client status
```

#### Disconnect

Disconnect from the L2TP server:

```bash
sudo l2tp-client disconnect
```

#### List

List all active L2TP tunnels and sessions:

```bash
sudo l2tp-client list
```

## Configuration File

You can use a configuration file (TOML format) instead of command-line flags:

```toml
server = "vpn.example.com"
port = 1701
username = "myuser"
password = "mypass"
auth = "pap"
interface = "ppp0"
ipv4 = true
ipv6 = false
auto-reconnect = true
reconnect-delay = 5
hello-timeout = 60
```

Use it with:

```bash
sudo l2tp-client connect --config /path/to/config.toml
```

## Examples

### Connect with CHAP authentication

```bash
sudo l2tp-client connect \
  --server vpn.example.com \
  --user myuser \
  --password mypass \
  --auth chap
```

### Connect with auto-reconnection

```bash
sudo l2tp-client connect \
  --server vpn.example.com \
  --user myuser \
  --password mypass \
  --auto-reconnect \
  --reconnect-delay 10
```

### Connect with IPv6 support

```bash
sudo l2tp-client connect \
  --server vpn.example.com \
  --user myuser \
  --password mypass \
  --ipv6
```

## Architecture

The client is built on top of the [go-l2tp](https://github.com/katalix/go-l2tp) library, which provides:

- L2TPv2 control plane for tunnel and session management
- Linux kernel L2TP subsystem integration via netlink
- PPP integration for authentication and network configuration

### Components

- **CLI Interface** (`cmd/l2tp-client/`): Command-line interface using Cobra
- **Client Library** (`internal/client/`): L2TP client wrapper and connection management
- **Configuration** (`internal/config/`): Configuration loading and validation
- **PPP Management** (`internal/ppp/`): pppd process management and integration

## Development

### Building

```bash
go build -o l2tp-client ./cmd/l2tp-client
```

### Testing

Run unit tests:

```bash
go test ./...
```

Run tests requiring root (for kernel integration):

```bash
sudo go test -exec sudo -run TestRequiresRoot ./...
```

### Dependencies

- `github.com/katalix/go-l2tp` - L2TP protocol implementation
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management

## Limitations

- Currently uses static tunnel mode - full control plane implementation for dynamic tunnels is pending
- Requires root privileges for kernel operations
- Linux-only (uses Linux-specific L2TP kernel subsystem)

## Security Considerations

- Passwords should be provided via environment variables or secure input, not plaintext in config files
- The client requires root privileges for L2TP kernel operations
- Consider using IPSec for additional tunnel security

## License

This project is licensed under the MIT License.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- Built on top of [go-l2tp](https://github.com/katalix/go-l2tp) by Katalix Systems
- Uses the Linux kernel L2TP subsystem
