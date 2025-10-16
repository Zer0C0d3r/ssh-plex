package target

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Target represents a parsed host specification containing connection details
type Target struct {
	User         string // SSH username
	Host         string // Hostname or IP address
	Port         int    // SSH port number
	IdentityFile string // Path to SSH private key file
	Original     string // Original host specification string
}

// Parser defines the interface for parsing and validating host specifications
type Parser interface {
	// ParseHosts parses comma-separated host specifications
	ParseHosts(input string) ([]Target, error)
	
	// ParseHostFile reads host specifications from a file (one per line)
	ParseHostFile(filename string) ([]Target, error)
	
	// ParseStdin reads host specifications from stdin
	ParseStdin() ([]Target, error)
	
	// ValidateTarget validates a target for security and correctness
	ValidateTarget(target Target) error
}

// DefaultParser implements the Parser interface
type DefaultParser struct{}

// NewParser creates a new DefaultParser instance
func NewParser() Parser {
	return &DefaultParser{}
}

// ParseHostSpec parses a single host specification in the format "user@host:port?key=path"
func ParseHostSpec(spec string) (Target, error) {
	target := Target{
		Original: spec,
		Port:     22, // Default SSH port
	}

	// Handle empty spec
	if strings.TrimSpace(spec) == "" {
		return target, fmt.Errorf("empty host specification")
	}

	// Split on '?' to separate host part from query parameters
	parts := strings.SplitN(spec, "?", 2)
	hostPart := parts[0]

	// Parse query parameters if present
	if len(parts) == 2 {
		queryPart := parts[1]
		values, err := url.ParseQuery(queryPart)
		if err != nil {
			return target, fmt.Errorf("invalid query parameters: %w", err)
		}
		
		if key := values.Get("key"); key != "" {
			target.IdentityFile = key
		}
	}

	// Parse user@host:port format
	var userHost string
	if strings.Contains(hostPart, "@") {
		userHostParts := strings.SplitN(hostPart, "@", 2)
		target.User = userHostParts[0]
		userHost = userHostParts[1]
	} else {
		userHost = hostPart
	}

	// Parse host:port
	var host string
	var portStr string
	
	// Handle IPv6 addresses in brackets
	if strings.HasPrefix(userHost, "[") {
		// IPv6 format: [::1]:2222
		closeBracket := strings.Index(userHost, "]")
		if closeBracket == -1 {
			return target, fmt.Errorf("invalid IPv6 address format: missing closing bracket")
		}
		
		host = userHost[1:closeBracket] // Remove brackets
		remainder := userHost[closeBracket+1:]
		
		if strings.HasPrefix(remainder, ":") {
			portStr = remainder[1:]
		}
	} else {
		// IPv4 or hostname format
		if strings.Contains(userHost, ":") {
			hostPortParts := strings.SplitN(userHost, ":", 2)
			host = hostPortParts[0]
			portStr = hostPortParts[1]
		} else {
			host = userHost
		}
	}

	target.Host = host

	// Parse port if specified
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return target, fmt.Errorf("invalid port number '%s': %w", portStr, err)
		}
		if port < 1 || port > 65535 {
			return target, fmt.Errorf("port number %d out of valid range (1-65535)", port)
		}
		target.Port = port
	}

	// Validate the target
	if err := ValidateTarget(target); err != nil {
		return target, fmt.Errorf("validation failed: %w", err)
	}

	return target, nil
}

// ValidateTarget validates a target for security and correctness
func ValidateTarget(target Target) error {
	// Validate host is not empty
	if target.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	// Use net.JoinHostPort for security validation to prevent injection attacks
	hostPort := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	if hostPort == "" {
		return fmt.Errorf("invalid host:port combination")
	}

	// Validate identity file path if specified
	if target.IdentityFile != "" {
		// Ensure path is absolute or relative to current working directory
		if !filepath.IsAbs(target.IdentityFile) {
			// Convert to absolute path relative to current directory
			absPath, err := filepath.Abs(target.IdentityFile)
			if err != nil {
				return fmt.Errorf("invalid identity file path '%s': %w", target.IdentityFile, err)
			}
			target.IdentityFile = absPath
		}

		// Check if file exists and is readable
		if _, err := os.Stat(target.IdentityFile); err != nil {
			return fmt.Errorf("identity file '%s' not accessible: %w", target.IdentityFile, err)
		}
	}

	return nil
}

// ParseHosts parses comma-separated host specifications
func (p *DefaultParser) ParseHosts(input string) ([]Target, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("empty hosts input")
	}

	specs := strings.Split(input, ",")
	targets := make([]Target, 0, len(specs))

	for i, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue // Skip empty entries
		}

		target, err := ParseHostSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("error parsing host %d ('%s'): %w", i+1, spec, err)
		}

		targets = append(targets, target)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no valid hosts found in input")
	}

	return targets, nil
}

// ParseHostFile reads host specifications from a file (one per line)
func (p *DefaultParser) ParseHostFile(filename string) ([]Target, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open host file '%s': %w", filename, err)
	}
	defer file.Close()

	return p.parseFromReader(file)
}

// ParseStdin reads host specifications from stdin
func (p *DefaultParser) ParseStdin() ([]Target, error) {
	return p.parseFromReader(os.Stdin)
}

// parseFromReader reads host specifications from any io.Reader (one per line)
func (p *DefaultParser) parseFromReader(reader io.Reader) ([]Target, error) {
	scanner := bufio.NewScanner(reader)
	targets := make([]Target, 0)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		target, err := ParseHostSpec(line)
		if err != nil {
			return nil, fmt.Errorf("error parsing line %d ('%s'): %w", lineNum, line, err)
		}

		targets = append(targets, target)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no valid hosts found in input")
	}

	return targets, nil
}

// ValidateTarget validates a target for security and correctness
func (p *DefaultParser) ValidateTarget(target Target) error {
	return ValidateTarget(target)
}