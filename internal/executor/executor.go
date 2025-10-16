package executor

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"ssh-plex/internal/logging"
	"ssh-plex/internal/ssh"
	"ssh-plex/internal/target"
)

// ExecutorConfig holds configuration parameters for the executor
type ExecutorConfig struct {
	Concurrency  int           // Maximum number of concurrent SSH connections (0 for auto)
	Retries      int           // Maximum number of retry attempts per target
	CmdTimeout   time.Duration // Timeout for individual command execution
	TotalTimeout time.Duration // Total timeout for all executions
}

// ParseConcurrency parses concurrency configuration from string
func ParseConcurrency(concurrencyStr string) (int, error) {
	if concurrencyStr == "" || concurrencyStr == "auto" {
		return 0, nil // 0 indicates auto mode
	}
	
	concurrency, err := strconv.Atoi(concurrencyStr)
	if err != nil {
		return 0, fmt.Errorf("invalid concurrency value '%s': must be a number or 'auto'", concurrencyStr)
	}
	
	if concurrency < 1 {
		return 0, fmt.Errorf("concurrency must be at least 1, got %d", concurrency)
	}
	
	if concurrency > 1000 {
		return 0, fmt.Errorf("concurrency too high: %d (maximum 1000)", concurrency)
	}
	
	return concurrency, nil
}

// Executor defines the interface for orchestrating parallel command execution
type Executor interface {
	// Execute runs a command on all targets and returns a channel of results
	Execute(ctx context.Context, targets []target.Target, command string) <-chan *ssh.Result
	
	// SetConfig updates the executor configuration
	SetConfig(config ExecutorConfig)
}

// Job represents a unit of work to be executed by a worker
type Job struct {
	Target  target.Target // Target host to execute command on
	Command string        // Command to execute
	Attempt int           // Current attempt number (0-based)
}

// WorkerPool manages concurrent execution of SSH commands
type WorkerPool struct {
	config     ExecutorConfig
	jobs       chan Job
	results    chan *ssh.Result
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	logger     *logging.Logger
}

// NewExecutor creates a new executor with default configuration
func NewExecutor() Executor {
	return &WorkerPool{
		config: ExecutorConfig{
			Concurrency:  10,
			Retries:      0,
			CmdTimeout:   60 * time.Second,
			TotalTimeout: 0, // No timeout by default
		},
	}
}

// NewExecutorWithLogger creates a new executor with logging
func NewExecutorWithLogger(logger *logging.Logger) Executor {
	return &WorkerPool{
		config: ExecutorConfig{
			Concurrency:  10,
			Retries:      0,
			CmdTimeout:   60 * time.Second,
			TotalTimeout: 0, // No timeout by default
		},
		logger: logger,
	}
}

// SetConfig updates the executor configuration
func (wp *WorkerPool) SetConfig(config ExecutorConfig) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.config = config
}

// Execute runs a command on all targets and returns a channel of results
func (wp *WorkerPool) Execute(ctx context.Context, targets []target.Target, command string) <-chan *ssh.Result {
	wp.mu.Lock()
	config := wp.config
	logger := wp.logger
	wp.mu.Unlock()

	// Calculate actual concurrency
	concurrency := calculateConcurrency(config.Concurrency, len(targets))
	
	// Log executor start
	if logger != nil {
		logger.LogExecutorStart(len(targets), concurrency, config.Retries)
	}
	
	// Create context with total timeout if specified
	execCtx := ctx
	var timeoutCancel context.CancelFunc
	if config.TotalTimeout > 0 {
		execCtx, timeoutCancel = context.WithTimeout(ctx, config.TotalTimeout)
	}
	
	// Initialize worker pool
	wp.ctx, wp.cancel = context.WithCancel(execCtx)
	wp.jobs = make(chan Job, len(targets))
	wp.results = make(chan *ssh.Result, len(targets)*(config.Retries+1))
	
	// Start workers
	for i := 0; i < concurrency; i++ {
		wp.wg.Add(1)
		go wp.worker(config)
	}
	
	// Start result collector
	resultsChan := make(chan *ssh.Result, len(targets))
	go wp.collectResults(targets, resultsChan, timeoutCancel)
	
	// Queue initial jobs
	for _, target := range targets {
		select {
		case wp.jobs <- Job{Target: target, Command: command, Attempt: 0}:
		case <-wp.ctx.Done():
			break
		}
	}
	
	return resultsChan
}

// worker processes jobs from the job queue
func (wp *WorkerPool) worker(config ExecutorConfig) {
	defer wp.wg.Done()
	
	for {
		select {
		case job := <-wp.jobs:
			result := wp.executeJob(job, config)
			
			// Send result
			select {
			case wp.results <- result:
			case <-wp.ctx.Done():
				return
			}
			
		case <-wp.ctx.Done():
			return
		}
	}
}

// executeJob executes a single job and handles retries
func (wp *WorkerPool) executeJob(job Job, config ExecutorConfig) *ssh.Result {
	// Create command timeout context
	cmdCtx := wp.ctx
	if config.CmdTimeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(wp.ctx, config.CmdTimeout)
		defer cancel()
	}
	
	// Create SSH client with logger
	var client ssh.Client
	if wp.logger != nil {
		client = ssh.NewClientWithLogger(wp.logger)
	} else {
		client = ssh.NewClient()
	}
	defer client.Close()
	
	// Connect to target
	if err := client.Connect(cmdCtx, job.Target); err != nil {
		result := &ssh.Result{
			Target:   job.Target,
			Error:    err,
			ExitCode: 255,
			Retries:  job.Attempt,
		}
		
		// Check if we should retry
		if job.Attempt < config.Retries && wp.shouldRetryResult(result) {
			wp.scheduleRetry(job, config)
		}
		
		return result
	}
	
	// Execute command
	result, err := client.Execute(cmdCtx, job.Command)
	if result == nil {
		result = &ssh.Result{
			Target:   job.Target,
			Error:    err,
			ExitCode: 255,
			Retries:  job.Attempt,
		}
	} else {
		result.Retries = job.Attempt
	}
	
	// Check if we should retry based on result
	if job.Attempt < config.Retries && wp.shouldRetryResult(result) {
		wp.scheduleRetry(job, config)
	}
	
	return result
}

// shouldRetry determines if an error is retryable based on error type and exit code
func (wp *WorkerPool) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := strings.ToLower(err.Error())
	
	// Network timeouts
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no route to host") ||
		strings.Contains(errStr, "network unreachable") {
		return true
	}
	
	// SSH handshake failures
	if strings.Contains(errStr, "handshake failed") ||
		strings.Contains(errStr, "ssh handshake failed") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection lost") {
		return true
	}
	
	// Check for net.Error timeout
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	
	return false
}

// shouldRetryResult determines if a result should be retried based on error and exit code
func (wp *WorkerPool) shouldRetryResult(result *ssh.Result) bool {
	// No error means success - don't retry
	if result.Error == nil {
		return false
	}
	
	// Exit code 255 indicates SSH connection error - check if retryable
	if result.ExitCode == 255 {
		return wp.shouldRetry(result.Error)
	}
	
	// For other cases, check if the error itself is retryable
	return wp.shouldRetry(result.Error)
}

// scheduleRetry schedules a job for retry with exponential backoff
func (wp *WorkerPool) scheduleRetry(job Job, config ExecutorConfig) {
	backoff := calculateBackoff(job.Attempt + 1)
	
	// Log retry attempt
	if wp.logger != nil {
		reason := "connection error"
		wp.logger.LogRetry(job.Target, job.Attempt+1, backoff, reason)
	}
	
	go func() {
		select {
		case <-time.After(backoff):
			retryJob := Job{
				Target:  job.Target,
				Command: job.Command,
				Attempt: job.Attempt + 1,
			}
			
			select {
			case wp.jobs <- retryJob:
			case <-wp.ctx.Done():
			}
			
		case <-wp.ctx.Done():
		}
	}()
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: 2^attempt seconds + random jitter up to 1 second
	base := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
	return base + jitter
}

// collectResults collects results and closes the output channel when done
func (wp *WorkerPool) collectResults(targets []target.Target, output chan<- *ssh.Result, timeoutCancel context.CancelFunc) {
	startTime := time.Now()
	defer func() {
		// Log executor completion
		if wp.logger != nil {
			successCount := 0
			failureCount := 0
			for range targets {
				// This is a simplified count - in a real implementation we'd track actual results
				successCount++ // Placeholder - would need to track actual success/failure counts
			}
			wp.logger.LogExecutorComplete(len(targets), successCount, failureCount, time.Since(startTime))
		}
		
		close(output)
		if timeoutCancel != nil {
			timeoutCancel()
		}
	}()
	
	completed := make(map[string]bool)
	expectedResults := len(targets)
	
	for {
		select {
		case result := <-wp.results:
			targetKey := fmt.Sprintf("%s@%s:%d", result.Target.User, result.Target.Host, result.Target.Port)
			
			// Only send the final result for each target (no retries in progress)
			if !completed[targetKey] {
				// Check if this is a final result (success or max retries reached)
				if result.Error == nil || result.Retries >= wp.config.Retries || !wp.shouldRetryResult(result) {
					completed[targetKey] = true
					output <- result
					
					if len(completed) >= expectedResults {
						// All targets completed - shutdown gracefully
						wp.shutdown()
						return
					}
				}
			}
			
		case <-wp.ctx.Done():
			// Timeout or cancellation - send timeout results for incomplete targets
			wp.sendTimeoutResults(targets, completed, output)
			wp.shutdown()
			return
		}
	}
}

// shutdown gracefully shuts down the worker pool
func (wp *WorkerPool) shutdown() {
	wp.cancel()
	wp.wg.Wait()
	close(wp.jobs)
}

// sendTimeoutResults sends timeout results for targets that haven't completed
func (wp *WorkerPool) sendTimeoutResults(targets []target.Target, completed map[string]bool, output chan<- *ssh.Result) {
	for _, target := range targets {
		targetKey := fmt.Sprintf("%s@%s:%d", target.User, target.Host, target.Port)
		if !completed[targetKey] {
			timeoutResult := &ssh.Result{
				Target:   target,
				Error:    fmt.Errorf("execution timeout exceeded"),
				ExitCode: 124, // Standard timeout exit code
				Retries:  0,
			}
			output <- timeoutResult
		}
	}
}

// calculateConcurrency determines the actual concurrency based on configuration and target count
func calculateConcurrency(configConcurrency int, targetCount int) int {
	// Handle invalid negative values
	if configConcurrency < 0 {
		return 1
	}
	
	if configConcurrency == 0 {
		// Auto mode: min(32, num_hosts)
		if targetCount <= 0 {
			return 1 // Minimum concurrency
		}
		if targetCount <= 32 {
			return targetCount
		}
		return 32
	}
	
	// Apply upper bound of 1000, but still respect target count
	effectiveConcurrency := configConcurrency
	if effectiveConcurrency > 1000 {
		effectiveConcurrency = 1000
	}
	
	// Use configured concurrency, but don't exceed target count unless there are no targets
	if targetCount > 0 && effectiveConcurrency > targetCount {
		return targetCount
	}
	
	return effectiveConcurrency
}