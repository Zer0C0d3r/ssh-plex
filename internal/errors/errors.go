// Package errors provides error classification and handling for ssh-plex.
package errors

import (
	"fmt"
	"strings"
)

// ErrorType represents the classification of errors
type ErrorType int

const (
	// SetupErrorType represents configuration, validation, or initialization errors
	SetupErrorType ErrorType = iota

	// ConnectionErrorType represents network or SSH connection errors
	ConnectionErrorType

	// AuthenticationErrorType represents SSH authentication failures
	AuthenticationErrorType

	// ExecutionErrorType represents command execution errors
	ExecutionErrorType

	// TimeoutErrorType represents timeout-related errors
	TimeoutErrorType

	// UnknownErrorType represents unclassified errors
	UnknownErrorType
)

// String returns a string representation of the error type
func (et ErrorType) String() string {
	switch et {
	case SetupErrorType:
		return "setup"
	case ConnectionErrorType:
		return "connection"
	case AuthenticationErrorType:
		return "authentication"
	case ExecutionErrorType:
		return "execution"
	case TimeoutErrorType:
		return "timeout"
	case UnknownErrorType:
		return "unknown"
	default:
		return "unknown"
	}
}

// ClassifiedError wraps an error with classification information
type ClassifiedError struct {
	Type      ErrorType
	Original  error
	Message   string
	Retryable bool
}

// Error implements the error interface
func (ce *ClassifiedError) Error() string {
	if ce.Message != "" {
		return ce.Message
	}
	if ce.Original != nil {
		return ce.Original.Error()
	}
	return "unknown error"
}

// Unwrap returns the original error for error unwrapping
func (ce *ClassifiedError) Unwrap() error {
	return ce.Original
}

// IsRetryable returns whether this error type should be retried
func (ce *ClassifiedError) IsRetryable() bool {
	return ce.Retryable
}

// ClassifyError analyzes an error and returns its classification
func ClassifyError(err error) *ClassifiedError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Setup/Configuration errors (not retryable)
	if isSetupError(errStr) {
		return &ClassifiedError{
			Type:      SetupErrorType,
			Original:  err,
			Retryable: false,
		}
	}

	// Authentication errors (not retryable)
	if isAuthenticationError(errStr) {
		return &ClassifiedError{
			Type:      AuthenticationErrorType,
			Original:  err,
			Retryable: false,
		}
	}

	// Timeout errors (retryable)
	if isTimeoutError(errStr) {
		return &ClassifiedError{
			Type:      TimeoutErrorType,
			Original:  err,
			Retryable: true,
		}
	}

	// Connection errors (retryable)
	if isConnectionError(errStr) {
		return &ClassifiedError{
			Type:      ConnectionErrorType,
			Original:  err,
			Retryable: true,
		}
	}

	// Execution errors (not retryable - these are command failures)
	if isExecutionError(errStr) {
		return &ClassifiedError{
			Type:      ExecutionErrorType,
			Original:  err,
			Retryable: false,
		}
	}

	// Unknown errors (not retryable by default for safety)
	return &ClassifiedError{
		Type:      UnknownErrorType,
		Original:  err,
		Retryable: false,
	}
}

// isSetupError checks if an error is related to setup/configuration
func isSetupError(errStr string) bool {
	setupKeywords := []string{
		"configuration",
		"invalid",
		"file not found",
		"directory not found",
		"parse error",
		"validation failed",
		"missing required",
		"unsupported",
		"malformed",
	}

	// Check for setup-specific "not found" errors (but not command execution errors)
	if strings.Contains(errStr, "not found") && !strings.Contains(errStr, "command not found") {
		return true
	}

	// Check for permission denied in file/config context (not SSH auth context)
	if strings.Contains(errStr, "permission denied") &&
		(strings.Contains(errStr, "file") || strings.Contains(errStr, "config") || strings.Contains(errStr, "directory")) {
		return true
	}

	for _, keyword := range setupKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isAuthenticationError checks if an error is related to SSH authentication
func isAuthenticationError(errStr string) bool {
	authKeywords := []string{
		"authentication failed",
		"auth fail",
		"permission denied (publickey)",
		"no supported authentication methods",
		"key exchange failed",
		"hostkey verification failed",
		"unable to authenticate",
		"invalid user",
		"access denied",
		"login incorrect",
	}

	for _, keyword := range authKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if an error is related to timeouts
func isTimeoutError(errStr string) bool {
	timeoutKeywords := []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"context deadline exceeded",
		"i/o timeout",
		"read timeout",
		"write timeout",
		"connection timeout",
	}

	for _, keyword := range timeoutKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isConnectionError checks if an error is related to network connectivity
func isConnectionError(errStr string) bool {
	connectionKeywords := []string{
		"connection refused",
		"connection reset",
		"connection lost",
		"connection closed",
		"network unreachable",
		"no route to host",
		"host unreachable",
		"broken pipe",
		"connection aborted",
		"handshake failed",
		"ssh handshake failed",
		"protocol error",
		"unexpected eof",
		"connection dropped",
	}

	for _, keyword := range connectionKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isExecutionError checks if an error is related to command execution
func isExecutionError(errStr string) bool {
	executionKeywords := []string{
		"command not found",
		"no such command",
		"execution failed",
		"process exited",
		"signal:",
		"killed",
		"terminated",
	}

	for _, keyword := range executionKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// NewSetupError creates a new setup error
func NewSetupError(message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Type:      SetupErrorType,
		Original:  original,
		Message:   message,
		Retryable: false,
	}
}

// NewConnectionError creates a new connection error
func NewConnectionError(message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Type:      ConnectionErrorType,
		Original:  original,
		Message:   message,
		Retryable: true,
	}
}

// NewAuthenticationError creates a new authentication error
func NewAuthenticationError(message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Type:      AuthenticationErrorType,
		Original:  original,
		Message:   message,
		Retryable: false,
	}
}

// NewExecutionError creates a new execution error
func NewExecutionError(message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Type:      ExecutionErrorType,
		Original:  original,
		Message:   message,
		Retryable: false,
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(message string, original error) *ClassifiedError {
	return &ClassifiedError{
		Type:      TimeoutErrorType,
		Original:  original,
		Message:   message,
		Retryable: true,
	}
}

// ErrorCollector collects and categorizes multiple errors
type ErrorCollector struct {
	errors map[ErrorType][]error
	count  int
}

// NewErrorCollector creates a new error collector
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{
		errors: make(map[ErrorType][]error),
	}
}

// Add adds an error to the collector
func (ec *ErrorCollector) Add(err error) {
	if err == nil {
		return
	}

	classified := ClassifyError(err)
	ec.errors[classified.Type] = append(ec.errors[classified.Type], err)
	ec.count++
}

// Count returns the total number of errors
func (ec *ErrorCollector) Count() int {
	return ec.count
}

// CountByType returns the number of errors of a specific type
func (ec *ErrorCollector) CountByType(errorType ErrorType) int {
	return len(ec.errors[errorType])
}

// HasErrors returns true if there are any errors
func (ec *ErrorCollector) HasErrors() bool {
	return ec.count > 0
}

// HasErrorsOfType returns true if there are errors of the specified type
func (ec *ErrorCollector) HasErrorsOfType(errorType ErrorType) bool {
	return len(ec.errors[errorType]) > 0
}

// GetErrorsByType returns all errors of a specific type
func (ec *ErrorCollector) GetErrorsByType(errorType ErrorType) []error {
	return ec.errors[errorType]
}

// Summary returns a summary of all collected errors
func (ec *ErrorCollector) Summary() string {
	if ec.count == 0 {
		return "no errors"
	}

	var parts []string
	for errorType, errors := range ec.errors {
		if len(errors) > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", len(errors), errorType.String()))
		}
	}

	return fmt.Sprintf("total: %d errors (%s)", ec.count, strings.Join(parts, ", "))
}

// ShouldFailFast returns true if any non-retryable errors exist that should cause immediate failure
func (ec *ErrorCollector) ShouldFailFast() bool {
	// Fail fast on setup errors - these indicate fundamental configuration issues
	return ec.HasErrorsOfType(SetupErrorType)
}
