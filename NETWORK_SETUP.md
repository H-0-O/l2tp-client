# Network Setup for Dependency Download

If you're experiencing network timeouts when running `go mod tidy` or `go mod download`, you may need to configure your Go proxy settings.

## Common Solutions

### 1. Check Network Connectivity

Ensure you have internet access:
```bash
ping proxy.golang.org
```

### 2. Configure Go Proxy

If you're behind a corporate firewall or have proxy requirements:

```bash
# Set Go proxy (if needed)
export GOPROXY=https://proxy.golang.org,direct

# Or use direct mode (bypasses proxy)
export GOPROXY=direct

# Or use a local proxy/mirror
export GOPROXY=https://goproxy.cn,direct  # China mirror
```

### 3. Use Go Modules Proxy Environment Variables

```bash
# Disable checksum verification temporarily (not recommended for production)
export GOSUMDB=off

# Or use a different checksum database
export GOSUMDB=sum.golang.org
```

### 4. Manual Dependency Download

If automatic download fails, you can try:

```bash
# Download dependencies manually
go get github.com/katalix/go-l2tp@v0.1.8
go get github.com/spf13/cobra@v1.8.0
go get github.com/spf13/viper@v1.18.2

# Then run tidy
go mod tidy
```

### 5. Offline Mode (if you have vendor directory)

If you have a vendor directory from another machine:

```bash
go mod vendor
go build -mod=vendor -o l2tp-client ./cmd/l2tp-client
```

## Troubleshooting

### Error: "dial tcp ... i/o timeout"

This indicates a network connectivity issue. Try:
1. Check your internet connection
2. Check firewall settings
3. Configure proxy if behind corporate firewall
4. Try using `GOPROXY=direct` to bypass proxy

### Error: "missing go.sum entry"

Run:
```bash
go mod tidy
```

This will download dependencies and update go.sum.

### Error: "module ... not found"

Ensure the module path is correct and the module exists. Check:
- GitHub repository is accessible
- Module version tag exists
- Network connectivity to GitHub/proxy

## Alternative: Use Go Workspace

If you have the dependencies locally:

```bash
go work init
go work use .
```

Then build normally.
