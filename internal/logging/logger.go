package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"ssh-plex/internal/target"
)

// LogLevel represents the logging level
type LogLevel string

const (
	LevelInfo  LogLevel = "info"
	LevelError LogLevel = "error"
)

// LogFormat represents the output format for logs
type LogFormat string

const (
	FormatJSON LogFormat = "json"
	FormatText LogFormat = "text"
)

// Config holds logging configuration
type Config struct {
	Level  LogLevel  // Minimum log level to output
	Format LogFormat // Output format (json or text)
	Output io.Writer // Output destination (defaults to stderr)
	Quiet  bool      // If true, suppress non-error output
}

// Logger wraps slog.Logger with secure logging practices
type Logger struct {
	logger *slog.Logger
	config Config
}

// NewLogger creates a new secure logger instance
func NewLogger(config Config) *Logger {
	// Set default output to stderr if not specified
	if config.Output == nil {
		config.Output = os.Stderr
	}

	// Create handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: convertLogLevel(config.Level),
	}

	switch config.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(config.Output, opts)
	case FormatText:
		handler = slog.NewTextHandler(config.Output, opts)
	default:
		// Default to text format
		handler = slog.NewTextHandler(config.Output, opts)
	}

	return &Logger{
		logger: slog.New(handler),
		config: config,
	}
}

// convertLogLevel converts our LogLevel to slog.Level
func convertLogLevel(level LogLevel) slog.Level {
	switch level {
	case LevelError:
		return slog.LevelError
	case LevelInfo:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

// Info logs an informational message
func (l *Logger) Info(msg string, args ...any) {
	if l.config.Quiet {
		return // Suppress non-error output in quiet mode
	}
	l.logger.Info(msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// InfoContext logs an informational message with context
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	if l.config.Quiet {
		return // Suppress non-error output in quiet mode
	}
	l.logger.InfoContext(ctx, msg, args...)
}

// ErrorContext logs an error message with context
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

// LogConnection logs SSH connection information securely
func (l *Logger) LogConnection(target target.Target, duration time.Duration, attempt int) {
	l.Info("ssh connection established",
		"host", target.Host,
		"user", target.User,
		"port", target.Port,
		"duration_ms", duration.Milliseconds(),
		"attempt", attempt,
		// Note: Never log identity file paths or authentication details
	)
}

// LogConnectionError logs SSH connection errors securely
func (l *Logger) LogConnectionError(target target.Target, err error, attempt int) {
	l.Error("ssh connection failed",
		"host", target.Host,
		"user", target.User,
		"port", target.Port,
		"error", err.Error(),
		"attempt", attempt,
		// Note: Never log identity file paths or authentication details
	)
}

// LogExecution logs command execution information
func (l *Logger) LogExecution(target target.Target, command string, exitCode int, duration time.Duration, attempt int) {
	l.Info("command executed",
		"host", target.Host,
		"user", target.User,
		"port", target.Port,
		"exit_code", exitCode,
		"duration_ms", duration.Milliseconds(),
		"attempt", attempt,
		// Note: Never log the actual command for security reasons
	)
}

// LogExecutionError logs command execution errors
func (l *Logger) LogExecutionError(target target.Target, command string, err error, attempt int) {
	l.Error("command execution failed",
		"host", target.Host,
		"user", target.User,
		"port", target.Port,
		"error", err.Error(),
		"attempt", attempt,
		// Note: Never log the actual command for security reasons
	)
}

// LogRetry logs retry attempt information
func (l *Logger) LogRetry(target target.Target, attempt int, backoff time.Duration, reason string) {
	l.Info("retrying connection",
		"host", target.Host,
		"user", target.User,
		"port", target.Port,
		"attempt", attempt,
		"backoff_ms", backoff.Milliseconds(),
		"reason", reason,
	)
}

// LogConnectionWarning logs security warnings for connections
func (l *Logger) LogConnectionWarning(hostname string, message string) {
	l.logger.Warn("connection security warning",
		"host", hostname,
		"warning", message,
	)
}

// LogExecutorStart logs the start of executor operations
func (l *Logger) LogExecutorStart(targetCount int, concurrency int, retries int) {
	l.Info("executor started",
		"target_count", targetCount,
		"concurrency", concurrency,
		"max_retries", retries,
	)
}

// LogExecutorComplete logs the completion of executor operations
func (l *Logger) LogExecutorComplete(targetCount int, successCount int, failureCount int, duration time.Duration) {
	l.Info("executor completed",
		"target_count", targetCount,
		"success_count", successCount,
		"failure_count", failureCount,
		"total_duration_ms", duration.Milliseconds(),
	)
}

// LogConfigLoad logs configuration loading events
func (l *Logger) LogConfigLoad(source string) {
	l.Info("configuration loaded",
		"source", source,
	)
}

// LogConfigError logs configuration errors
func (l *Logger) LogConfigError(source string, err error) {
	l.Error("configuration error",
		"source", source,
		"error", err.Error(),
	)
}

// LogTargetParsing logs target parsing information
func (l *Logger) LogTargetParsing(source string, count int) {
	l.Info("targets parsed",
		"source", source,
		"count", count,
	)
}

// LogTargetParsingError logs target parsing errors
func (l *Logger) LogTargetParsingError(source string, err error) {
	l.Error("target parsing failed",
		"source", source,
		"error", err.Error(),
	)
}

// WithContext returns a new logger that includes context values in log entries
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// For now, return the same logger. In the future, we could extract
	// context values like request IDs, trace IDs, etc.
	return l
}

// SetLevel updates the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.config.Level = level
	// Note: slog doesn't support runtime level changes easily,
	// so this would require recreating the handler
}

// IsQuiet returns whether the logger is in quiet mode
func (l *Logger) IsQuiet() bool {
	return l.config.Quiet
}

// NewLoggerFromConfig creates a logger from application configuration
func NewLoggerFromConfig(logLevel, logFormat string, quiet bool) *Logger {
	var level LogLevel
	switch logLevel {
	case "error":
		level = LevelError
	case "info":
		level = LevelInfo
	default:
		level = LevelInfo // Default to info level
	}

	var format LogFormat
	switch logFormat {
	case "json":
		format = FormatJSON
	case "text":
		format = FormatText
	default:
		format = FormatText // Default to text format
	}

	config := Config{
		Level:  level,
		Format: format,
		Quiet:  quiet,
	}

	return NewLogger(config)
}
