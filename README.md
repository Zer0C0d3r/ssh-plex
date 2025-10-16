# ssh-plex

[![CI/CD Pipeline](https://github.com/Zer0C0d3r/ssh-plex/workflows/CI%2FCD%20Pipeline/badge.svg)](https://github.com/Zer0C0d3r/ssh-plex/actions)
[![CodeQL](https://github.com/Zer0C0d3r/ssh-plex/workflows/CodeQL%20Security%20Analysis/badge.svg)](https://github.com/Zer0C0d3r/ssh-plex/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/Zer0C0d3r/ssh-plex)](https://goreportcard.com/report/github.com/Zer0C0d3r/ssh-plex)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/github/release/Zer0C0d3r/ssh-plex.svg)](https://github.com/Zer0C0d3r/ssh-plex/releases/latest)

A high-reliability, production-grade CLI tool that enables parallel, fault-tolerant execution of shell commands across multiple remote hosts via SSH. Built for DevOps engineers, SREs, and system administrators who need to manage infrastructure at scale.

## ✨ Features

- **🚀 Parallel Execution**: Execute commands across hundreds of hosts simultaneously with configurable concurrency
- **🔄 Smart Retry Logic**: Automatic retry with exponential backoff for transient failures
- **📊 Multiple Output Formats**: Streamed, buffered, and JSON output modes for different use cases
- **⚡ High Performance**: Static binary with minimal resource footprint
- **🛡️ Production Ready**: Comprehensive error handling, logging, and observability
- **🔧 Flexible Configuration**: Command-line flags, environment variables, and configuration files
- **📝 Rich Logging**: Structured logging with configurable levels and formats
- **🎯 Dynamic Targeting**: Support for host files, command-line lists, and stdin input
- **🔐 SSH Authentication**: Support for SSH agent, custom SSH keys, and default key authentication
- **⏱️ Timeout Controls**: Per-command and total execution timeouts
- **🏃 Dry Run Mode**: Preview execution plans without making changes

## 📦 Installation

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/Zer0C0d3r/ssh-plex/releases):

```bash
# Linux (x86_64)
curl -L https://github.com/Zer0C0d3r/ssh-plex/releases/latest/download/ssh-plex-linux-amd64.tar.gz | tar xz
sudo mv ssh-plex /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/Zer0C0d3r/ssh-plex/releases/latest/download/ssh-plex-darwin-amd64.tar.gz | tar xz
sudo mv ssh-plex /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/Zer0C0d3r/ssh-plex/releases/latest/download/ssh-plex-darwin-arm64.tar.gz | tar xz
sudo mv ssh-plex /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/Zer0C0d3r/ssh-plex/releases/latest/download/ssh-plex-windows-amd64.zip" -OutFile "ssh-plex.zip"
Expand-Archive ssh-plex.zip
```

### Build from Source

```bash
# Prerequisites: Go 1.22+
git clone https://github.com/Zer0C0d3r/ssh-plex.git
cd ssh-plex
make build

# Or install directly
go install github.com/Zer0C0d3r/ssh-plex/cmd/ssh-plex@latest
```

## 🚀 Quick Start

### Basic Usage

```bash
# Execute a command on multiple hosts
ssh-plex --hosts "user@host1,user@host2,user@host3" -- uptime

# Use a host file
echo -e "user@server1\nuser@server2\nuser@server3" > hosts.txt
ssh-plex --hostfile hosts.txt -- "df -h"

# Read hosts from stdin
echo -e "user@host1\nuser@host2" | ssh-plex -- "systemctl status nginx"
```

### Advanced Examples

```bash
# High concurrency with retries and JSON output
ssh-plex --hosts "user@host1,user@host2" \
         --concurrency 20 \
         --retries 3 \
         --output json \
         --timeout 5m \
         -- "docker ps"

# Dry run to preview execution plan
ssh-plex --hosts "user@host1,user@host2" \
         --dry-run \
         -- "systemctl restart nginx"

# Custom SSH key and logging
ssh-plex --hosts "user@host1?key=/path/to/key.pem,user@host2:2222" \
         --log-level error \
         --log-format json \
         -- "tail -n 100 /var/log/app.log"
```

## 📖 Usage Guide

### Command Syntax

```bash
ssh-plex [flags] -- <command>
```

**Important**: The `--` separator is required before the command to execute.

### Host Specification Formats

ssh-plex supports flexible host specification formats:

```bash
# Basic formats
user@hostname
user@hostname:port
hostname  # Uses current user

# With custom SSH key
user@hostname?key=/path/to/key.pem
user@hostname:2222?key=/path/to/key.pem

# IPv6 support
user@[2001:db8::1]:22
user@[2001:db8::1]?key=/path/to/key.pem
```

**Authentication Priority:**
1. Custom SSH key (if specified with `?key=path`)
2. SSH agent (if `SSH_AUTH_SOCK` is available)
3. Default SSH keys (`~/.ssh/id_rsa`, `~/.ssh/id_ed25519`, etc.)

### Configuration Methods

ssh-plex supports multiple configuration methods with the following precedence (highest to lowest):

1. **Command-line flags**
2. **Environment variables** (prefixed with `SSH_PLEX_`)
3. **Configuration files** (`config.yaml` only)

#### Environment Variables

```bash
export SSH_PLEX_CONCURRENCY=10
export SSH_PLEX_RETRIES=2
export SSH_PLEX_TIMEOUT=300s
export SSH_PLEX_OUTPUT=json
export SSH_PLEX_LOG_LEVEL=info
```

#### Configuration File Example

Configuration files are searched in the following locations:
- `/etc/ssh-plex/config.yaml` (system-wide)
- `~/.config/ssh-plex/config.yaml` (user-specific)

**Note**: Currently only YAML format is supported. JSON and TOML support is planned for v1.1.

```yaml
# config.yaml
concurrency: "auto"
retries: 2
timeout: "5m"
cmd-timeout: "60s"
output: "streamed"
log-level: "info"
log-format: "text"
quiet: false
```

## 🔧 Command Reference

### Global Flags

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--hosts` | `SSH_PLEX_HOSTS` | - | Comma-separated list of host specifications |
| `--hostfile` | `SSH_PLEX_HOSTFILE` | - | Path to file containing host specifications |
| `--concurrency` | `SSH_PLEX_CONCURRENCY` | `10` | Max concurrent connections (`auto` or number) |
| `--retries` | `SSH_PLEX_RETRIES` | `0` | Maximum retry attempts per target |
| `--timeout` | `SSH_PLEX_TIMEOUT` | `0` | Total execution timeout (0 for unlimited) |
| `--cmd-timeout` | `SSH_PLEX_CMD_TIMEOUT` | `60s` | Per-command timeout |
| `--output` | `SSH_PLEX_OUTPUT` | `streamed` | Output format (`streamed`, `buffered`, `json`) |
| `--quiet` | `SSH_PLEX_QUIET` | `false` | Suppress non-error output |
| `--dry-run` | `SSH_PLEX_DRY_RUN` | `false` | Show execution plan without connecting |
| `--log-level` | `SSH_PLEX_LOG_LEVEL` | `info` | Log level (`info`, `error`) |
| `--log-format` | `SSH_PLEX_LOG_FORMAT` | `text` | Log format (`text`, `json`) |

### Commands

#### `ssh-plex [flags] -- <command>`

Execute a shell command across multiple SSH hosts.

**Examples:**
```bash
# Basic execution
ssh-plex --hosts "user@host1,user@host2" -- "uptime"

# With custom settings
ssh-plex --hosts "user@host1,user@host2" \
         --concurrency 5 \
         --retries 2 \
         --timeout 2m \
         -- "systemctl status nginx"
```

**Troubleshooting:**
- **Error: "command is required after '--'"** - Ensure you include `--` before your command
- **Error: "must specify hosts"** - Provide hosts via `--hosts`, `--hostfile`, or stdin
- **Connection timeouts** - Increase `--cmd-timeout` or check network connectivity
- **Authentication failures** - Verify SSH keys and user permissions

#### `ssh-plex version`

Display version information.

**Example:**
```bash
ssh-plex version
# Output:
# ssh-plex v1.0.0
# Commit: abc123
# Built: 2024-01-01T00:00:00Z
```

### Output Formats

#### Streamed Mode (Default)
Real-time output as commands execute:
```
[host1] stdout: System uptime: 5 days
[host2] stdout: System uptime: 12 days
[host1] completed (exit: 0, duration: 1.2s)
[host2] completed (exit: 0, duration: 1.1s)
```

#### Buffered Mode
Organized output after all commands complete:
```
=== host1 ===
System uptime: 5 days
Exit Code: 0
Duration: 1.2s

=== host2 ===
System uptime: 12 days
Exit Code: 0
Duration: 1.1s
```

#### JSON Mode
Machine-readable output for automation:
```json
{
  "target": {"host": "host1", "user": "user", "port": 22},
  "command": "uptime",
  "stdout": "System uptime: 5 days\n",
  "stderr": "",
  "exit_code": 0,
  "duration": "1.2s",
  "retries": 0,
  "error": null
}
```

## ⚠️ Current Limitations

- **Configuration Files**: Only YAML format supported (JSON/TOML planned for v1.1)
- **Config Locations**: Only system (`/etc/ssh-plex/`) and user (`~/.config/ssh-plex/`) directories
- **Host Parameters**: Only `key=path` parameter supported in host specifications
- **Authentication**: No password authentication (SSH keys and agent only)

## 🔍 Troubleshooting

### Common Issues

#### Connection Problems

**Issue**: `connection refused` or `timeout` errors
```bash
# Solutions:
# 1. Verify host connectivity
ping hostname

# 2. Check SSH service
ssh user@hostname

# 3. Increase timeout
ssh-plex --cmd-timeout 120s --hosts "user@hostname" -- "command"

# 4. Use custom port
ssh-plex --hosts "user@hostname:2222" -- "command"
```

#### Authentication Issues

**Issue**: `permission denied` or `authentication failed`
```bash
# Solutions:
# 1. Ensure SSH agent is running (if using agent auth)
ssh-add -l

# 2. Use specific SSH key
ssh-plex --hosts "user@hostname?key=/path/to/key.pem" -- "command"

# 3. Verify key permissions
chmod 600 /path/to/key.pem

# 4. Test SSH connection manually
ssh -i /path/to/key.pem user@hostname

# 5. Check if SSH_AUTH_SOCK is set for agent authentication
echo $SSH_AUTH_SOCK
```

#### Performance Issues

**Issue**: Slow execution or high resource usage
```bash
# Solutions:
# 1. Reduce concurrency
ssh-plex --concurrency 5 --hosts "..." -- "command"

# 2. Use auto concurrency
ssh-plex --concurrency auto --hosts "..." -- "command"

# 3. Increase timeouts for slow commands
ssh-plex --cmd-timeout 300s --hosts "..." -- "slow-command"
```

#### Output Issues

**Issue**: Missing or garbled output
```bash
# Solutions:
# 1. Use buffered output for cleaner results
ssh-plex --output buffered --hosts "..." -- "command"

# 2. Use JSON for automation
ssh-plex --output json --hosts "..." -- "command"

# 3. Enable debug logging
ssh-plex --log-level info --hosts "..." -- "command"
```

### Exit Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 0 | Success | All targets executed successfully |
| 1 | Execution Failure | One or more targets failed |
| 2 | Setup Error | Configuration or setup issues |

### Debug Mode

Enable detailed logging for troubleshooting:

```bash
# Text format logging
ssh-plex --log-level info --log-format text --hosts "..." -- "command"

# JSON format logging
ssh-plex --log-level info --log-format json --hosts "..." -- "command"
```

## 🗺️ Roadmap

### Version 1.1.0
- [ ] **Enhanced Configuration**
  - Support for JSON and TOML config files (currently YAML only)
  - Config file support in current directory
  - Advanced query parameters in host specifications
- [ ] **Enhanced Authentication**
  - Password authentication option
  - Multi-factor authentication support
- [ ] **Improved Output**
  - Progress bars for long-running operations
  - Real-time statistics dashboard
  - Export results to CSV/Excel

### Version 1.2.0
- [ ] **Advanced Features**
  - Host grouping and tagging
  - Conditional execution based on host properties
  - Template support for complex commands
- [ ] **Integration**
  - Ansible inventory support
  - Kubernetes node targeting
  - Cloud provider integration (AWS, GCP, Azure)

### Version 1.3.0
- [ ] **Enterprise Features**
  - Role-based access control
  - Audit logging
  - Integration with secret management systems
- [ ] **Performance**
  - Connection pooling and reuse
  - Streaming large outputs
  - Memory optimization for large host lists

### Version 1.4.0
- [ ] **Architecture**
  - Plugin system for extensibility
  - Web UI for management
  - REST API for integration
- [ ] **Advanced Orchestration**
  - Workflow engine
  - Dependency management
  - Rollback capabilities

## 🤝 Contributing

We welcome contributions from the community! Here's how you can help:

### Ways to Contribute

- 🐛 **Report Bugs**: Open an issue with detailed reproduction steps
- 💡 **Suggest Features**: Share your ideas for new functionality
- 📝 **Improve Documentation**: Help make our docs clearer and more comprehensive
- 🔧 **Submit Code**: Fix bugs or implement new features
- 🧪 **Testing**: Help test new releases and report issues

### Development Setup

```bash
# Clone the repository
git clone https://github.com/Zer0C0d3r/ssh-plex.git
cd ssh-plex

# Install dependencies
go mod download

# Build the project
make build

# Run tests
make test

# Run linter
make lint
```

### Contribution Guidelines

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'Add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

### Code Standards

- Follow Go best practices and idioms
- Write comprehensive tests for new features
- Update documentation for user-facing changes
- Ensure all CI checks pass
- Use conventional commit messages

### Getting Help

- 💬 **Discussions**: Use GitHub Discussions for questions and ideas
- 🐛 **Issues**: Report bugs and request features via GitHub Issues
- 📧 **Email**: Contact maintainers at [odin.coder77@proton.me]

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 📊 Statistics

- **Language**: Go
- **Platforms**: Linux, macOS, Windows
- **Architectures**: amd64, arm64
- **Dependencies**: Minimal (only essential Go modules)
- **Binary Size**: ~8MB (statically linked)

---

**Made with ❤️ by Zer0C0d3r**

*Star ⭐ this repository if you find it useful!*