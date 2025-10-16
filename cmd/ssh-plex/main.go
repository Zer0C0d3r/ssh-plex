package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ssh-plex/internal/config"
	"ssh-plex/internal/errors"
	"ssh-plex/internal/executor"
	"ssh-plex/internal/logging"
	"ssh-plex/internal/output"
	"ssh-plex/internal/target"

	"github.com/spf13/cobra"
)

var (
	// Build-time variables (set via -ldflags)
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"

	// Global configuration
	cfg *config.Config

	// CLI flags
	hosts       string
	hostFile    string
	concurrency string
	retries     int
	timeout     time.Duration
	cmdTimeout  time.Duration
	outputMode  string
	quiet       bool
	dryRun      bool
	logLevel    string
	logFormat   string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(getExitCode(err))
	}
}

var rootCmd = &cobra.Command{
	Use:   "ssh-plex [flags] -- <command>",
	Short: "Execute commands in parallel across multiple SSH hosts",
	Long: `ssh-plex is a high-reliability, production-grade CLI tool that enables 
parallel, fault-tolerant execution of shell commands across multiple remote 
hosts via SSH.

The tool provides advanced features for observability, retry logic, dynamic 
targeting, and structured output, making it suitable for CI/CD pipelines, 
SRE runbooks, and production incident response.

Examples:
  # Execute command on hosts from command line
  ssh-plex --hosts "user@host1,user@host2" -- uptime

  # Execute command on hosts from file
  ssh-plex --hostfile hosts.txt -- "df -h"

  # Execute with custom concurrency and retries
  ssh-plex --hosts "user@host1,user@host2" --concurrency 5 --retries 2 -- "systemctl status nginx"

  # Use JSON output for automation
  ssh-plex --hosts "user@host1,user@host2" --output json -- "hostname"

  # Dry run to see execution plan
  ssh-plex --hosts "user@host1,user@host2" --dry-run -- "echo test"`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return &SetupError{Message: "command is required after '--'"}
		}
		return nil
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration from all sources
		configManager := config.NewManager()
		loadedCfg, err := configManager.Load()
		if err != nil {
			return &SetupError{Message: fmt.Sprintf("failed to load configuration: %v", err)}
		}
		cfg = loadedCfg

		// Override config with CLI flags if provided
		if err := overrideConfigWithFlags(cmd); err != nil {
			return &SetupError{Message: fmt.Sprintf("failed to apply CLI flags: %v", err)}
		}

		// Validate that we have at least one host source
		if cfg.Hosts == "" && cfg.HostFile == "" && isStdinTTY() {
			return &SetupError{Message: "must specify hosts via --hosts, --hostfile, or stdin"}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Join all arguments to form the command
		command := strings.Join(args, " ")

		return executeCommand(command)
	},
}

func init() {
	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ssh-plex %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built: %s\n", buildTime)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Add all CLI flags
	rootCmd.Flags().StringVar(&hosts, "hosts", "", "Comma-separated list of host specifications (user@host:port?key=path)")
	rootCmd.Flags().StringVar(&hostFile, "hostfile", "", "Path to file containing host specifications (one per line)")
	rootCmd.Flags().StringVar(&concurrency, "concurrency", "10", "Maximum concurrent connections ('auto' or number)")
	rootCmd.Flags().IntVar(&retries, "retries", 0, "Maximum retry attempts per target")
	rootCmd.Flags().DurationVar(&timeout, "timeout", 0, "Total execution timeout (0 for no timeout)")
	rootCmd.Flags().DurationVar(&cmdTimeout, "cmd-timeout", 60*time.Second, "Per-command timeout")
	rootCmd.Flags().StringVar(&outputMode, "output", "streamed", "Output format (streamed, buffered, json)")
	rootCmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress non-error output")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show execution plan without connecting")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (info, error)")
	rootCmd.Flags().StringVar(&logFormat, "log-format", "text", "Log format (json, text)")

	// Mark the command as requiring the -- separator
	rootCmd.SetUsageTemplate(rootCmd.UsageTemplate() + `
Note: Command to execute must be specified after '--' separator.
`)
}

func overrideConfigWithFlags(cmd *cobra.Command) error {
	// Override configuration with CLI flags if they were explicitly set
	if cmd.Flags().Changed("hosts") {
		cfg.Hosts = hosts
	}
	if cmd.Flags().Changed("hostfile") {
		cfg.HostFile = hostFile
	}
	if cmd.Flags().Changed("concurrency") {
		cfg.Concurrency = concurrency
	}
	if cmd.Flags().Changed("retries") {
		cfg.Retries = retries
	}
	if cmd.Flags().Changed("timeout") {
		cfg.Timeout = timeout
	}
	if cmd.Flags().Changed("cmd-timeout") {
		cfg.CmdTimeout = cmdTimeout
	}
	if cmd.Flags().Changed("output") {
		cfg.Output = outputMode
	}
	if cmd.Flags().Changed("quiet") {
		cfg.Quiet = quiet
	}
	if cmd.Flags().Changed("dry-run") {
		cfg.DryRun = dryRun
	}
	if cmd.Flags().Changed("log-level") {
		cfg.LogLevel = logLevel
	}
	if cmd.Flags().Changed("log-format") {
		cfg.LogFormat = logFormat
	}

	// Validate the final configuration
	configManager := config.NewManager()
	if err := configManager.Validate(cfg); err != nil {
		return &SetupError{Message: fmt.Sprintf("configuration validation failed: %v", err)}
	}

	return nil
}

func executeCommand(command string) error {
	return executeCommandInternal(command, os.Stdout)
}

func executeCommandInternal(command string, writer io.Writer) error {
	// Set up logging with proper error handling
	logger := logging.NewLoggerFromConfig(cfg.LogLevel, cfg.LogFormat, cfg.Quiet)
	if logger == nil {
		return &SetupError{Message: "failed to initialize logger"}
	}

	// Log configuration loading
	logger.LogConfigLoad("CLI flags and configuration files")

	// Parse targets with comprehensive error handling
	parser := target.NewParser()
	var targets []target.Target
	var err error
	var source string

	if cfg.Hosts != "" {
		source = "CLI hosts parameter"
		targets, err = parser.ParseHosts(cfg.Hosts)
		if err != nil {
			logger.LogTargetParsingError(source, err)
			return &SetupError{Message: fmt.Sprintf("failed to parse hosts: %v", err)}
		}
	} else if cfg.HostFile != "" {
		source = fmt.Sprintf("host file: %s", cfg.HostFile)
		targets, err = parser.ParseHostFile(cfg.HostFile)
		if err != nil {
			logger.LogTargetParsingError(source, err)
			return &SetupError{Message: fmt.Sprintf("failed to parse host file: %v", err)}
		}
	} else {
		source = "stdin"
		targets, err = parser.ParseStdin()
		if err != nil {
			logger.LogTargetParsingError(source, err)
			return &SetupError{Message: fmt.Sprintf("failed to parse hosts from stdin: %v", err)}
		}
	}

	if len(targets) == 0 {
		logger.LogTargetParsingError(source, fmt.Errorf("no valid targets found"))
		return &SetupError{Message: "no valid targets found"}
	}

	// Log successful target parsing
	logger.LogTargetParsing(source, len(targets))

	// Handle dry-run mode
	if cfg.DryRun {
		return performDryRun(targets, command, logger, writer)
	}

	// Set up output formatter with error handling
	var outputMode output.OutputMode
	switch cfg.Output {
	case "streamed":
		outputMode = output.StreamedMode
	case "buffered":
		outputMode = output.BufferedMode
	case "json":
		outputMode = output.JSONMode
	default:
		return &SetupError{Message: fmt.Sprintf("invalid output mode: %s", cfg.Output)}
	}

	formatter := output.NewFormatter(outputMode, writer)
	if formatter == nil {
		return &SetupError{Message: "failed to initialize output formatter"}
	}

	// Calculate concurrency with validation
	concurrencyValue, err := calculateConcurrency(cfg.Concurrency, len(targets))
	if err != nil {
		logger.LogConfigError("concurrency calculation", err)
		return &SetupError{Message: fmt.Sprintf("failed to calculate concurrency: %v", err)}
	}

	// Create executor with proper error handling
	exec := executor.NewExecutorWithLogger(logger)
	if exec == nil {
		return &SetupError{Message: "failed to initialize executor"}
	}

	// Set up executor configuration
	executorConfig := executor.ExecutorConfig{
		Concurrency:  concurrencyValue,
		Retries:      cfg.Retries,
		CmdTimeout:   cfg.CmdTimeout,
		TotalTimeout: cfg.Timeout,
	}
	exec.SetConfig(executorConfig)

	// Set up context with proper cancellation handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Add total timeout if specified
	if cfg.Timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, cfg.Timeout)
		defer timeoutCancel()
	}

	// Set up graceful shutdown handling for SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start a goroutine to handle shutdown signals
	go func() {
		select {
		case sig := <-sigChan:
			logger.Info("Received shutdown signal, canceling operations", "signal", sig.String())
			cancel() // Cancel the context to stop all operations
		case <-ctx.Done():
			// Context already canceled, nothing to do
		}
	}()
	defer signal.Stop(sigChan)

	// Execute commands with comprehensive error handling
	results := exec.Execute(ctx, targets, command)
	if results == nil {
		return &SetupError{Message: "executor failed to start"}
	}

	// Process results and track execution statistics with comprehensive error handling
	var (
		hasFailures  bool
		totalTargets int
		successCount int
		failureCount int
	)

	// Use error collector for comprehensive error tracking
	errorCollector := errors.NewErrorCollector()

	// Process results with proper error classification
	for result := range results {
		totalTargets++

		// Format output with error handling - never crash on formatting errors
		if err := formatter.Format(result); err != nil {
			logger.Error("Failed to format output", "error", err, "host", result.Target.Host)
			// Formatting errors don't affect execution success but should be logged
		}

		// Classify result based on error type and exit code
		if result.Error != nil {
			// Classify the error for proper handling
			classifiedError := errors.ClassifyError(result.Error)
			errorCollector.Add(result.Error)
			hasFailures = true

			logger.Error("Target error",
				"host", result.Target.Host,
				"error", result.Error,
				"retries", result.Retries,
				"error_type", classifiedError.Type.String(),
				"retryable", classifiedError.IsRetryable())
		} else if result.ExitCode != 0 {
			failureCount++
			hasFailures = true
			logger.Info("Target command failed",
				"host", result.Target.Host,
				"exit_code", result.ExitCode,
				"retries", result.Retries)
		} else {
			successCount++
			logger.Info("Target execution successful",
				"host", result.Target.Host,
				"duration_ms", result.Duration.Milliseconds(),
				"retries", result.Retries)
		}

		// Check for fail-fast conditions (never crash on individual host failures)
		if errorCollector.ShouldFailFast() && errorCollector.CountByType(errors.SetupErrorType) > (totalTargets/2) {
			// If more than half the targets have setup errors, log warning but continue
			logger.Error("High rate of setup errors detected, but continuing execution",
				"setup_errors", errorCollector.CountByType(errors.SetupErrorType),
				"total_processed", totalTargets)
		}
	}

	// Finalize output with error handling
	if err := formatter.Finalize(); err != nil {
		logger.Error("Failed to finalize output", "error", err)
		// Don't fail the entire execution for output formatting issues
	}

	// Log comprehensive execution summary with error breakdown
	logger.Info("Execution completed",
		"total_targets", totalTargets,
		"successful", successCount,
		"failed_commands", failureCount,
		"error_summary", errorCollector.Summary(),
		"setup_errors", errorCollector.CountByType(errors.SetupErrorType),
		"connection_errors", errorCollector.CountByType(errors.ConnectionErrorType),
		"auth_errors", errorCollector.CountByType(errors.AuthenticationErrorType),
		"execution_errors", errorCollector.CountByType(errors.ExecutionErrorType),
		"timeout_errors", errorCollector.CountByType(errors.TimeoutErrorType))

	// Return appropriate exit code based on results - never crash the application
	if hasFailures {
		totalErrors := errorCollector.Count()
		return &ExecutionError{
			Message: fmt.Sprintf("execution failed: %d/%d targets failed (%d errors, %d non-zero exits) - %s",
				totalErrors+failureCount, totalTargets, totalErrors, failureCount, errorCollector.Summary()),
		}
	}

	return nil
}

func performDryRun(targets []target.Target, command string, logger *logging.Logger, writer io.Writer) error {
	fmt.Fprintln(writer, "ssh-plex Dry Run - Execution Plan")
	fmt.Fprintln(writer, "=================================")
	fmt.Fprintln(writer)

	// Display configuration details
	fmt.Fprintln(writer, "Configuration:")
	fmt.Fprintf(writer, "  Command: %s\n", command)
	fmt.Fprintf(writer, "  Total Targets: %d\n", len(targets))
	fmt.Fprintf(writer, "  Concurrency Setting: %s\n", cfg.Concurrency)
	fmt.Fprintf(writer, "  Max Retries: %d\n", cfg.Retries)
	fmt.Fprintf(writer, "  Command Timeout: %v\n", cfg.CmdTimeout)
	if cfg.Timeout > 0 {
		fmt.Fprintf(writer, "  Total Timeout: %v\n", cfg.Timeout)
	} else {
		fmt.Fprintf(writer, "  Total Timeout: unlimited\n")
	}
	fmt.Fprintf(writer, "  Output Format: %s\n", cfg.Output)
	fmt.Fprintf(writer, "  Log Level: %s\n", cfg.LogLevel)
	fmt.Fprintf(writer, "  Log Format: %s\n", cfg.LogFormat)
	fmt.Fprintf(writer, "  Quiet Mode: %t\n", cfg.Quiet)
	fmt.Fprintln(writer)

	// Calculate and display resolved concurrency
	concurrencyValue, err := calculateConcurrency(cfg.Concurrency, len(targets))
	if err != nil {
		return fmt.Errorf("failed to calculate concurrency: %w", err)
	}
	fmt.Fprintf(writer, "Execution Plan:\n")
	fmt.Fprintf(writer, "  Resolved Concurrency: %d workers\n", concurrencyValue)

	// Calculate estimated execution batches
	batches := (len(targets) + concurrencyValue - 1) / concurrencyValue
	fmt.Fprintf(writer, "  Execution Batches: %d\n", batches)

	// Estimate execution time (rough calculation)
	estimatedTime := time.Duration(batches) * cfg.CmdTimeout
	if cfg.Retries > 0 {
		// Factor in potential retries (conservative estimate)
		retryFactor := 1.0 + (float64(cfg.Retries) * 0.3) // Assume 30% of commands might need retries
		estimatedTime = time.Duration(float64(estimatedTime) * retryFactor)
	}
	fmt.Fprintf(writer, "  Estimated Max Duration: %v (excluding network latency)\n", estimatedTime)
	fmt.Fprintln(writer)

	// Display target details
	fmt.Fprintf(writer, "Target Details:\n")
	for i, target := range targets {
		fmt.Fprintf(writer, "  %d. %s\n", i+1, target.Original)
		fmt.Fprintf(writer, "     → User: %s, Host: %s, Port: %d\n", target.User, target.Host, target.Port)
		if target.IdentityFile != "" {
			fmt.Fprintf(writer, "     → Identity File: %s\n", target.IdentityFile)
		} else {
			fmt.Fprintf(writer, "     → Authentication: SSH agent or default keys\n")
		}
	}
	fmt.Fprintln(writer)

	// Display execution flow
	fmt.Fprintf(writer, "Execution Flow:\n")
	fmt.Fprintf(writer, "  1. Initialize %d worker goroutines\n", concurrencyValue)
	fmt.Fprintf(writer, "  2. Distribute %d targets across workers\n", len(targets))
	fmt.Fprintf(writer, "  3. Execute command on each target with %v timeout\n", cfg.CmdTimeout)
	if cfg.Retries > 0 {
		fmt.Fprintf(writer, "  4. Retry failed targets up to %d times with exponential backoff\n", cfg.Retries)
	}
	fmt.Fprintf(writer, "  5. Collect and format results in '%s' mode\n", cfg.Output)
	fmt.Fprintf(writer, "  6. Return exit code based on overall success/failure\n")
	fmt.Fprintln(writer)

	// Display what would happen without actually connecting
	fmt.Fprintf(writer, "Note: This is a dry run. No SSH connections will be established.\n")
	fmt.Fprintf(writer, "To execute for real, remove the --dry-run flag.\n")

	return nil
}

func calculateConcurrency(concurrencyStr string, targetCount int) (int, error) {
	if concurrencyStr == "auto" {
		if targetCount < 32 {
			return targetCount, nil
		}
		return 32, nil
	}

	concurrency, err := strconv.Atoi(concurrencyStr)
	if err != nil {
		return 0, &SetupError{Message: fmt.Sprintf("invalid concurrency value '%s': must be 'auto' or a positive integer", concurrencyStr)}
	}

	if concurrency <= 0 {
		return 0, &SetupError{Message: fmt.Sprintf("concurrency must be positive, got %d", concurrency)}
	}

	return concurrency, nil
}

func isStdinTTY() bool {
	// Check if stdin is a TTY (terminal)
	stat, err := os.Stdin.Stat()
	if err != nil {
		return true // Assume TTY on error
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// ExecutionError represents an error during command execution (exit code 1)
type ExecutionError struct {
	Message string
}

func (e *ExecutionError) Error() string {
	return e.Message
}

// SetupError represents an error during setup/configuration (exit code 2)
type SetupError struct {
	Message string
}

func (e *SetupError) Error() string {
	return e.Message
}

// getExitCode determines the appropriate exit code based on error type
// Returns:
//   - 0: Success (all targets succeeded)
//   - 1: Execution failure (one or more targets failed)
//   - 2: Setup error (invalid arguments, configuration issues, etc.)
func getExitCode(err error) int {
	if err == nil {
		return 0 // Success
	}

	switch err.(type) {
	case *SetupError:
		return 2 // Setup/configuration errors
	case *ExecutionError:
		return 1 // Command execution failures
	default:
		// Unknown errors are treated as setup errors for safety
		// This includes panics, unexpected errors, etc.
		return 2
	}
}
