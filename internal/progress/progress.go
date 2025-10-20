// Package progress provides progress tracking and display for ssh-plex operations.
package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ProgressTracker tracks and displays progress for long-running operations
type ProgressTracker struct {
	total     int
	completed int
	failed    int
	startTime time.Time
	mu        sync.RWMutex
	writer    io.Writer
	enabled   bool
	lastDraw  time.Time
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(total int, writer io.Writer, enabled bool) *ProgressTracker {
	return &ProgressTracker{
		total:     total,
		completed: 0,
		failed:    0,
		startTime: time.Now(),
		writer:    writer,
		enabled:   enabled,
	}
}

// Update increments the progress counters
func (p *ProgressTracker) Update(success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if success {
		p.completed++
	} else {
		p.failed++
	}

	if p.enabled {
		p.draw()
	}
}

// Finish completes the progress tracking and shows final stats
func (p *ProgressTracker) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.enabled {
		p.drawFinal()
	}
}

// draw renders the current progress bar
func (p *ProgressTracker) draw() {
	now := time.Now()
	// Throttle updates to avoid excessive output
	if now.Sub(p.lastDraw) < 100*time.Millisecond {
		return
	}
	p.lastDraw = now

	total := p.completed + p.failed
	if p.total == 0 {
		return
	}

	percentage := float64(total) / float64(p.total) * 100
	elapsed := now.Sub(p.startTime)

	// Calculate ETA
	var eta string
	if total > 0 {
		avgTimePerTask := elapsed / time.Duration(total)
		remaining := p.total - total
		etaDuration := avgTimePerTask * time.Duration(remaining)
		eta = fmt.Sprintf("ETA: %v", etaDuration.Round(time.Second))
	} else {
		eta = "ETA: calculating..."
	}

	// Create progress bar
	barWidth := 40
	filled := int(float64(barWidth) * percentage / 100)
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// Format: [████████████░░░░░░░░] 75% (15/20) ✓12 ✗3 [2m30s] ETA: 50s
	fmt.Fprintf(p.writer, "\r[%s] %.1f%% (%d/%d) ✓%d ✗%d [%v] %s",
		bar, percentage, total, p.total, p.completed, p.failed,
		elapsed.Round(time.Second), eta)
}

// drawFinal renders the final progress summary
func (p *ProgressTracker) drawFinal() {
	total := p.completed + p.failed
	elapsed := time.Since(p.startTime)

	fmt.Fprintf(p.writer, "\r")
	// Clear the line
	for i := 0; i < 100; i++ {
		fmt.Fprintf(p.writer, " ")
	}
	fmt.Fprintf(p.writer, "\r")

	// Final summary
	if p.failed == 0 {
		fmt.Fprintf(p.writer, "✓ Completed %d/%d tasks successfully in %v\n",
			p.completed, p.total, elapsed.Round(time.Second))
	} else {
		fmt.Fprintf(p.writer, "⚠ Completed %d/%d tasks (%d successful, %d failed) in %v\n",
			total, p.total, p.completed, p.failed, elapsed.Round(time.Second))
	}
}

// GetStats returns current progress statistics
func (p *ProgressTracker) GetStats() (completed, failed, total int, elapsed time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.completed, p.failed, p.total, time.Since(p.startTime)
}
