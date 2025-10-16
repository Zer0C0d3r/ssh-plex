package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"ssh-plex/internal/ssh"
)

// OutputMode defines the available output formatting modes
type OutputMode string

const (
	// StreamedMode outputs results in real-time with [host] prefixes
	StreamedMode OutputMode = "streamed"
	
	// BufferedMode shows complete output per host after execution completion
	BufferedMode OutputMode = "buffered"
	
	// JSONMode emits NDJSON objects with structured result data
	JSONMode OutputMode = "json"
)

// Formatter defines the interface for formatting and displaying command results
type Formatter interface {
	// Format processes and outputs a single result
	Format(result *ssh.Result) error
	
	// Finalize performs any cleanup or final output operations
	Finalize() error
	
	// SetMode configures the output formatting mode
	SetMode(mode OutputMode)
}

// DefaultFormatter implements the Formatter interface with support for all output modes
type DefaultFormatter struct {
	mode     OutputMode
	writer   io.Writer
	mu       sync.Mutex
	buffered map[string]*ssh.Result // For buffered mode
}

// NewFormatter creates a new formatter with the specified mode and writer
func NewFormatter(mode OutputMode, writer io.Writer) Formatter {
	if writer == nil {
		writer = os.Stdout
	}
	
	return &DefaultFormatter{
		mode:     mode,
		writer:   writer,
		buffered: make(map[string]*ssh.Result),
	}
}

// SetMode configures the output formatting mode
func (f *DefaultFormatter) SetMode(mode OutputMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

// Format processes and outputs a single result based on the current mode
func (f *DefaultFormatter) Format(result *ssh.Result) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	switch f.mode {
	case StreamedMode:
		return f.formatStreamed(result)
	case BufferedMode:
		return f.formatBuffered(result)
	case JSONMode:
		return f.formatJSON(result)
	default:
		return fmt.Errorf("unknown output mode: %s", f.mode)
	}
}

// Finalize performs any cleanup or final output operations
func (f *DefaultFormatter) Finalize() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	if f.mode == BufferedMode {
		return f.flushBuffered()
	}
	return nil
}

// formatStreamed outputs results in real-time with [host] prefixes
func (f *DefaultFormatter) formatStreamed(result *ssh.Result) error {
	hostPrefix := fmt.Sprintf("[%s]", result.Target.Host)
	
	// Output stdout with host prefix
	if result.Stdout != "" {
		scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
		for scanner.Scan() {
			line := scanner.Text()
			if _, err := fmt.Fprintf(f.writer, "%s %s\n", hostPrefix, line); err != nil {
				return fmt.Errorf("failed to write stdout: %w", err)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stdout: %w", err)
		}
	}
	
	// Output stderr with host prefix
	if result.Stderr != "" {
		scanner := bufio.NewScanner(strings.NewReader(result.Stderr))
		for scanner.Scan() {
			line := scanner.Text()
			if _, err := fmt.Fprintf(f.writer, "%s %s\n", hostPrefix, line); err != nil {
				return fmt.Errorf("failed to write stderr: %w", err)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading stderr: %w", err)
		}
	}
	
	// Output error information if present
	if result.Error != nil {
		if _, err := fmt.Fprintf(f.writer, "%s ERROR: %s (exit code: %d)\n", 
			hostPrefix, result.Error.Error(), result.ExitCode); err != nil {
			return fmt.Errorf("failed to write error: %w", err)
		}
	}
	
	return nil
}

// formatBuffered stores results for later display after all executions complete
func (f *DefaultFormatter) formatBuffered(result *ssh.Result) error {
	f.buffered[result.Target.Host] = result
	return nil
}

// flushBuffered outputs all buffered results in sorted order by hostname
func (f *DefaultFormatter) flushBuffered() error {
	// Sort hosts for consistent output
	hosts := make([]string, 0, len(f.buffered))
	for host := range f.buffered {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	
	// Output results for each host
	for i, host := range hosts {
		result := f.buffered[host]
		
		// Add separator between hosts (except for the first one)
		if i > 0 {
			if _, err := fmt.Fprintln(f.writer, ""); err != nil {
				return fmt.Errorf("failed to write separator: %w", err)
			}
		}
		
		// Host header
		if _, err := fmt.Fprintf(f.writer, "=== %s ===\n", host); err != nil {
			return fmt.Errorf("failed to write host header: %w", err)
		}
		
		// Output stdout
		if result.Stdout != "" {
			if _, err := fmt.Fprint(f.writer, result.Stdout); err != nil {
				return fmt.Errorf("failed to write stdout: %w", err)
			}
			// Ensure stdout ends with newline
			if !strings.HasSuffix(result.Stdout, "\n") {
				if _, err := fmt.Fprintln(f.writer, ""); err != nil {
					return fmt.Errorf("failed to write newline: %w", err)
				}
			}
		}
		
		// Output stderr
		if result.Stderr != "" {
			if _, err := fmt.Fprint(f.writer, result.Stderr); err != nil {
				return fmt.Errorf("failed to write stderr: %w", err)
			}
			// Ensure stderr ends with newline
			if !strings.HasSuffix(result.Stderr, "\n") {
				if _, err := fmt.Fprintln(f.writer, ""); err != nil {
					return fmt.Errorf("failed to write newline: %w", err)
				}
			}
		}
		
		// Output error and exit code information
		if result.Error != nil {
			if _, err := fmt.Fprintf(f.writer, "ERROR: %s\n", result.Error.Error()); err != nil {
				return fmt.Errorf("failed to write error: %w", err)
			}
		}
		
		if _, err := fmt.Fprintf(f.writer, "Exit code: %d, Duration: %v", 
			result.ExitCode, result.Duration); err != nil {
			return fmt.Errorf("failed to write exit info: %w", err)
		}
		
		if result.Retries > 0 {
			if _, err := fmt.Fprintf(f.writer, ", Retries: %d", result.Retries); err != nil {
				return fmt.Errorf("failed to write retry info: %w", err)
			}
		}
		
		if _, err := fmt.Fprintln(f.writer, ""); err != nil {
			return fmt.Errorf("failed to write final newline: %w", err)
		}
	}
	
	return nil
}

// JSONOutput represents the JSON structure for NDJSON output
type JSONOutput struct {
	Host        string `json:"host"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exit_code"`
	DurationMs  int64  `json:"duration_ms"`
	Retries     int    `json:"retries"`
	Error       string `json:"error,omitempty"`
}

// formatJSON outputs results as NDJSON (newline-delimited JSON)
func (f *DefaultFormatter) formatJSON(result *ssh.Result) error {
	output := JSONOutput{
		Host:       result.Target.Host,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		ExitCode:   result.ExitCode,
		DurationMs: result.Duration.Milliseconds(),
		Retries:    result.Retries,
	}
	
	if result.Error != nil {
		output.Error = result.Error.Error()
	}
	
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	
	if _, err := fmt.Fprintf(f.writer, "%s\n", jsonBytes); err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}
	
	return nil
}