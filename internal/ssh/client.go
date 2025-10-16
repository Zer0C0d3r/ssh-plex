package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"ssh-plex/internal/logging"
	"ssh-plex/internal/target"
)

// Result represents the outcome of executing a command on a target host
type Result struct {
	Target   target.Target // The target host where command was executed
	Stdout   string        // Standard output from the command
	Stderr   string        // Standard error from the command
	ExitCode int           // Exit code returned by the command
	Duration time.Duration // Time taken to execute the command
	Error    error         // Any error that occurred during execution
	Retries  int           // Number of retry attempts made
}

// Client defines the interface for SSH operations
type Client interface {
	// Connect establishes an SSH connection to the target host
	Connect(ctx context.Context, target target.Target) error

	// Execute runs a command on the connected host and returns the result
	Execute(ctx context.Context, command string) (*Result, error)

	// Close terminates the SSH connection
	Close() error
}

// SSHClient implements the Client interface using golang.org/x/crypto/ssh
type SSHClient struct {
	conn   *ssh.Client
	target target.Target
	config *ssh.ClientConfig
	logger *logging.Logger
}

// NewClient creates a new SSH client instance
func NewClient() Client {
	return &SSHClient{}
}

// NewClientWithLogger creates a new SSH client instance with logging
func NewClientWithLogger(logger *logging.Logger) Client {
	return &SSHClient{
		logger: logger,
	}
}

// Connect establishes an SSH connection to the target host
func (c *SSHClient) Connect(ctx context.Context, target target.Target) error {
	c.target = target
	startTime := time.Now()

	// Add panic recovery to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("SSH connection panic: %v", r)
			if c.logger != nil {
				c.logger.LogConnectionError(target, err, 0)
			}
		}
	}()

	// Build SSH client configuration with error handling
	config, err := c.buildSSHConfig(target)
	if err != nil {
		if c.logger != nil {
			c.logger.LogConnectionError(target, fmt.Errorf("failed to build SSH config: %w", err), 0)
		}
		return fmt.Errorf("failed to build SSH config: %w", err)
	}
	c.config = config

	// Create address string
	address := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))

	// Create a dialer with timeout support
	dialer := &net.Dialer{
		Timeout: 30 * time.Second, // Connection timeout
	}

	// Establish connection with context support
	netConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		if c.logger != nil {
			c.logger.LogConnectionError(target, fmt.Errorf("failed to connect to %s: %w", address, err), 0)
		}
		return fmt.Errorf("failed to connect to %s: %w", address, err)
	}

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, address, config)
	if err != nil {
		netConn.Close()
		if c.logger != nil {
			c.logger.LogConnectionError(target, fmt.Errorf("SSH handshake failed for %s: %w", address, err), 0)
		}
		return fmt.Errorf("SSH handshake failed for %s: %w", address, err)
	}

	// Create SSH client
	c.conn = ssh.NewClient(sshConn, chans, reqs)

	// Log successful connection
	if c.logger != nil {
		c.logger.LogConnection(target, time.Since(startTime), 0)
	}

	return nil
}

// Execute runs a command on the connected host and returns the result
func (c *SSHClient) Execute(ctx context.Context, command string) (*Result, error) {
	result := &Result{
		Target: c.target,
	}

	if c.conn == nil {
		result.Error = fmt.Errorf("not connected to any host")
		result.ExitCode = 255
		return result, result.Error
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)

		// Recover from any panics to prevent crashes
		if r := recover(); r != nil {
			result.Error = fmt.Errorf("SSH execution panic: %v", r)
			result.ExitCode = 255
			if c.logger != nil {
				c.logger.LogExecutionError(c.target, command, result.Error, 0)
			}
		}
	}()

	// Create a new session with error handling
	session, err := c.conn.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %w", err)
		result.ExitCode = 255
		return result, result.Error
	}
	defer func() {
		// Safely close session, ignoring errors
		if session != nil {
			_ = session.Close()
		}
	}()

	// Set up stdout and stderr capture with proper buffering
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Execute command with proper timeout handling
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		// Capture output regardless of error
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()

		if err != nil {
			// Check if it's an exit error to get the exit code
			if exitErr, ok := err.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
				// Log execution with non-zero exit code
				if c.logger != nil {
					c.logger.LogExecution(c.target, command, result.ExitCode, result.Duration, 0)
				}
				// Exit error is not considered a connection error
				return result, nil
			} else {
				// Other SSH errors (connection issues, etc.)
				result.Error = fmt.Errorf("SSH execution error: %w", err)
				result.ExitCode = 255 // SSH connection error code
				if c.logger != nil {
					c.logger.LogExecutionError(c.target, command, result.Error, 0)
				}
				return result, result.Error
			}
		} else {
			result.ExitCode = 0
			// Log successful execution
			if c.logger != nil {
				c.logger.LogExecution(c.target, command, result.ExitCode, result.Duration, 0)
			}
		}

		return result, nil

	case <-ctx.Done():
		// Context canceled (timeout or cancellation)
		// Try to terminate the session gracefully
		if session != nil {
			session.Signal(ssh.SIGTERM)
			// Give it a moment to terminate gracefully
			select {
			case <-done:
				// Command finished after SIGTERM
			case <-time.After(2 * time.Second):
				// Force kill if it doesn't respond to SIGTERM
				session.Signal(ssh.SIGKILL)
			}
			session.Close()
		}

		// Capture any output that was generated before timeout
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		result.Error = fmt.Errorf("command execution timeout: %w", ctx.Err())
		result.ExitCode = 124 // Standard timeout exit code

		// Log timeout error
		if c.logger != nil {
			c.logger.LogExecutionError(c.target, command, result.Error, 0)
		}

		return result, result.Error
	}
}

// Close terminates the SSH connection
func (c *SSHClient) Close() error {
	// Add panic recovery to prevent crashes during cleanup
	defer func() {
		if r := recover(); r != nil {
			if c.logger != nil {
				c.logger.Error("SSH close panic", "panic", r, "host", c.target.Host)
			}
		}
	}()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		// Don't return connection close errors as they're not critical
		// Just log them if we have a logger
		if err != nil && c.logger != nil {
			c.logger.Error("SSH connection close error", "error", err, "host", c.target.Host)
		}
	}
	return nil
}

// buildSSHConfig creates an SSH client configuration with authentication methods
func (c *SSHClient) buildSSHConfig(target target.Target) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:            target.User,
		HostKeyCallback: c.getHostKeyCallback(),
		Timeout:         30 * time.Second,
	}

	// Set up authentication methods
	authMethods, err := c.getAuthMethods(target)
	if err != nil {
		return nil, fmt.Errorf("failed to set up authentication: %w", err)
	}
	config.Auth = authMethods

	return config, nil
}

// getAuthMethods returns available authentication methods in order of preference
func (c *SSHClient) getAuthMethods(target target.Target) ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// 1. Try SSH agent authentication first
	if agentAuth := c.getAgentAuth(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
	}

	// 2. Try identity file authentication if specified
	if target.IdentityFile != "" {
		keyAuth, err := c.getKeyAuth(target.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load identity file %s: %w", target.IdentityFile, err)
		}
		authMethods = append(authMethods, keyAuth)
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	return authMethods, nil
}

// getAgentAuth returns SSH agent authentication if available
func (c *SSHClient) getAgentAuth() ssh.AuthMethod {
	if agentConn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		return ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers)
	}
	return nil
}

// getKeyAuth returns public key authentication using the specified private key file
func (c *SSHClient) getKeyAuth(keyPath string) (ssh.AuthMethod, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return ssh.PublicKeys(signer), nil
}

// getHostKeyCallback returns a secure host key callback that tries known_hosts first,
// then falls back to a warning-based insecure callback for development/testing
func (c *SSHClient) getHostKeyCallback() ssh.HostKeyCallback {
	// Try to use known_hosts file for secure host key verification
	if homeDir, err := os.UserHomeDir(); err == nil {
		knownHostsFile := homeDir + "/.ssh/known_hosts"
		if _, err := os.Stat(knownHostsFile); err == nil {
			if hostKeyCallback, err := knownhosts.New(knownHostsFile); err == nil {
				return hostKeyCallback
			}
		}
	}

	// Fallback to system known_hosts
	if hostKeyCallback, err := knownhosts.New("/etc/ssh/ssh_known_hosts"); err == nil {
		return hostKeyCallback
	}

	// Final fallback: insecure callback with warning
	// This is acceptable for tools that need to work across many unknown hosts
	return ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if c.logger != nil {
			c.logger.LogConnectionWarning(hostname, "Host key verification disabled - not recommended for production")
		}
		return nil
	})
}
