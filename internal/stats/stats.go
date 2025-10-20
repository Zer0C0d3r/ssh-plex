// Package stats provides real-time statistics tracking for ssh-plex operations.
package stats

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Statistics holds real-time execution statistics
type Statistics struct {
	StartTime        time.Time
	TotalHosts       int
	CompletedHosts   int
	FailedHosts      int
	ActiveHosts      int
	TotalCommands    int
	SuccessfulCmds   int
	FailedCmds       int
	TotalRetries     int
	BytesTransferred int64
	mu               sync.RWMutex
}

// StatsTracker provides real-time statistics tracking and display
type StatsTracker struct {
	stats   *Statistics
	writer  io.Writer
	enabled bool
	ticker  *time.Ticker
	done    chan bool
}

// NewStatsTracker creates a new statistics tracker
func NewStatsTracker(totalHosts int, writer io.Writer, enabled bool) *StatsTracker {
	return &StatsTracker{
		stats: &Statistics{
			StartTime:  time.Now(),
			TotalHosts: totalHosts,
		},
		writer:  writer,
		enabled: enabled,
		done:    make(chan bool),
	}
}

// Start begins real-time statistics display
func (st *StatsTracker) Start() {
	if !st.enabled {
		return
	}

	st.ticker = time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-st.ticker.C:
				st.displayStats()
			case <-st.done:
				return
			}
		}
	}()
}

// Stop stops the real-time statistics display
func (st *StatsTracker) Stop() {
	if st.ticker != nil {
		st.ticker.Stop()
		st.done <- true
	}

	if st.enabled {
		st.displayFinalStats()
	}
}

// UpdateHostStarted increments active hosts counter
func (st *StatsTracker) UpdateHostStarted() {
	st.stats.mu.Lock()
	defer st.stats.mu.Unlock()
	st.stats.ActiveHosts++
}

// UpdateHostCompleted updates host completion statistics
func (st *StatsTracker) UpdateHostCompleted(success bool, retries int, bytesTransferred int64) {
	st.stats.mu.Lock()
	defer st.stats.mu.Unlock()

	st.stats.ActiveHosts--
	st.stats.TotalCommands++
	st.stats.TotalRetries += retries
	st.stats.BytesTransferred += bytesTransferred

	if success {
		st.stats.CompletedHosts++
		st.stats.SuccessfulCmds++
	} else {
		st.stats.FailedHosts++
		st.stats.FailedCmds++
	}
}

// displayStats shows current statistics
func (st *StatsTracker) displayStats() {
	st.stats.mu.RLock()
	defer st.stats.mu.RUnlock()

	elapsed := time.Since(st.stats.StartTime)
	completed := st.stats.CompletedHosts + st.stats.FailedHosts

	// Calculate rates
	var hostsPerSec float64
	if elapsed.Seconds() > 0 {
		hostsPerSec = float64(completed) / elapsed.Seconds()
	}

	// Calculate ETA
	var eta string
	if hostsPerSec > 0 {
		remaining := st.stats.TotalHosts - completed
		etaSeconds := float64(remaining) / hostsPerSec
		eta = fmt.Sprintf("ETA: %v", time.Duration(etaSeconds)*time.Second)
	} else {
		eta = "ETA: calculating..."
	}

	// Format bytes transferred
	bytesStr := formatBytes(st.stats.BytesTransferred)

	// Clear previous line and display stats
	fmt.Fprintf(st.writer, "\r\033[K") // Clear line
	fmt.Fprintf(st.writer, "📊 Hosts: %d/%d (✓%d ✗%d ~%d) | Rate: %.1f h/s | Commands: %d | Retries: %d | Data: %s | %s | %v",
		completed, st.stats.TotalHosts,
		st.stats.CompletedHosts, st.stats.FailedHosts, st.stats.ActiveHosts,
		hostsPerSec, st.stats.TotalCommands, st.stats.TotalRetries,
		bytesStr, eta, elapsed.Round(time.Second))
}

// displayFinalStats shows final execution statistics
func (st *StatsTracker) displayFinalStats() {
	st.stats.mu.RLock()
	defer st.stats.mu.RUnlock()

	elapsed := time.Since(st.stats.StartTime)

	fmt.Fprintf(st.writer, "\r\033[K") // Clear line
	fmt.Fprintf(st.writer, "\n")
	fmt.Fprintf(st.writer, "📈 Final Statistics:\n")
	fmt.Fprintf(st.writer, "   Total Hosts: %d\n", st.stats.TotalHosts)
	fmt.Fprintf(st.writer, "   Successful: %d (%.1f%%)\n",
		st.stats.CompletedHosts,
		float64(st.stats.CompletedHosts)/float64(st.stats.TotalHosts)*100)
	fmt.Fprintf(st.writer, "   Failed: %d (%.1f%%)\n",
		st.stats.FailedHosts,
		float64(st.stats.FailedHosts)/float64(st.stats.TotalHosts)*100)
	fmt.Fprintf(st.writer, "   Total Commands: %d\n", st.stats.TotalCommands)
	fmt.Fprintf(st.writer, "   Total Retries: %d\n", st.stats.TotalRetries)
	fmt.Fprintf(st.writer, "   Data Transferred: %s\n", formatBytes(st.stats.BytesTransferred))
	fmt.Fprintf(st.writer, "   Execution Time: %v\n", elapsed.Round(time.Second))

	if elapsed.Seconds() > 0 {
		fmt.Fprintf(st.writer, "   Average Rate: %.2f hosts/second\n",
			float64(st.stats.TotalHosts)/elapsed.Seconds())
	}
	fmt.Fprintf(st.writer, "\n")
}

// GetStatistics returns a copy of current statistics
func (st *StatsTracker) GetStatistics() Statistics {
	st.stats.mu.RLock()
	defer st.stats.mu.RUnlock()

	// Return a copy without the mutex to avoid copylocks issue
	return Statistics{
		StartTime:        st.stats.StartTime,
		TotalHosts:       st.stats.TotalHosts,
		CompletedHosts:   st.stats.CompletedHosts,
		FailedHosts:      st.stats.FailedHosts,
		ActiveHosts:      st.stats.ActiveHosts,
		TotalCommands:    st.stats.TotalCommands,
		SuccessfulCmds:   st.stats.SuccessfulCmds,
		FailedCmds:       st.stats.FailedCmds,
		TotalRetries:     st.stats.TotalRetries,
		BytesTransferred: st.stats.BytesTransferred,
	}
}

// formatBytes formats byte count in human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
