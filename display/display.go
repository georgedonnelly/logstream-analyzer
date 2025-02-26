// display/display.go - Handles rendering the stats to the terminal

package display

import (
	"fmt"
	"sort"
	"time"

	"log_analyzer/models"
)

// Display handles rendering the stats to the terminal
type Display struct {
	statsChan     chan *models.LogStats
	alertChan     chan models.Alert
	stopChan      chan struct{}
	alerts        []models.Alert
	maxAlerts     int
	clearScreenFn func()
}

// NewDisplay creates a new Display
func NewDisplay(statsChan chan *models.LogStats, alertChan chan models.Alert) *Display {
	return &Display{
		statsChan:     statsChan,
		alertChan:     alertChan,
		stopChan:      make(chan struct{}),
		alerts:        make([]models.Alert, 0, 10),
		maxAlerts:     12, // Show the 12 most recent alerts
		clearScreenFn: clearScreen,
	}
}

// Start begins updating the display
func (d *Display) Start() {
	go d.collectAlerts()
	go d.updateDisplay()
}

// Stop signals the display to stop
func (d *Display) Stop() {
	close(d.stopChan)
}

func (d *Display) collectAlerts() {
	for {
		select {
		case <-d.stopChan:
			return
		case alert := <-d.alertChan:
			d.alerts = append(d.alerts, alert)
			if len(d.alerts) > 50 { // Keep a reasonable history
				d.alerts = d.alerts[1:]
			}
		}
	}
}

func (d *Display) updateDisplay() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopChan:
			return
		case stats := <-d.statsChan:
			d.render(stats)
		case <-ticker.C:
			// Just trigger refresh if needed
		}
	}
}

func (d *Display) render(stats *models.LogStats) {
	if stats == nil {
		return
	}

	d.clearScreenFn()

	// Format timestamp
	timestamp := stats.LastUpdated.UTC().Format("2006-01-02 15:04:05 UTC")

	// Format window size with previous window size if it changed
	windowSizeText := fmt.Sprintf("%d sec", stats.WindowSize)
	if stats.PreviousWindowSize > 0 && stats.PreviousWindowSize != stats.WindowSize {
		windowSizeText = fmt.Sprintf("%d sec (Adjusted from %d sec)", 
			stats.WindowSize, stats.PreviousWindowSize)
	}

	// Build the report
	report := fmt.Sprintf(`
Log Analysis Report (Last Updated: %s)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Runtime Stats:
• Entries Processed: %s
• Current Rate: %.0f entries/sec (Peak: %.0f entries/sec)
• Adaptive Window: %s

Pattern Analysis:`,
		timestamp,
		formatNumber(stats.EntriesProcessed),
		stats.CurrentRate,
		stats.PeakRate,
		windowSizeText,
	)

	// Add log level distribution
	totalLogs := 0
	for _, count := range stats.LevelCounts {
		totalLogs += count
	}

	if totalLogs > 0 {
		// Sort levels for consistent display
		levels := []string{"ERROR", "INFO", "DEBUG"}
		for _, level := range levels {
			count, ok := stats.LevelCounts[level]
			if ok {
				percentage := 100.0 * float64(count) / float64(totalLogs)
				report += fmt.Sprintf("\n• %s: %.0f%% (%s entries)", 
					level, percentage, formatNumber(count))
			}
		}
	}

	// Add Dynamic Insights section
	report += "\n\nDynamic Insights:"

	// Calculate total error rate
	totalErrorRate := 0.0
	for _, rate := range stats.ErrorRates {
		totalErrorRate += rate
	}

	// Add total error rate if we have errors
	if totalErrorRate > 0 {
		report += fmt.Sprintf("\n• Error Rate: %.1f errors/sec", totalErrorRate)
	}

	// Add emerging patterns if any
	if len(stats.EmergingPatterns) > 0 {
		// Sort patterns by change percentage
		var patterns []struct {
			Name    string
			Change  float64
		}
		
		for pattern, change := range stats.EmergingPatterns {
			patterns = append(patterns, struct {
				Name    string
				Change  float64
			}{pattern, change})
		}
		
		sort.Slice(patterns, func(i, j int) bool {
			return patterns[i].Change > patterns[j].Change
		})
		
		// Take the top pattern
		if len(patterns) > 0 {
			report += fmt.Sprintf("\n• Emerging Pattern: \"%s\" spiked %.0f%% in last 15 sec",
				patterns[0].Name, patterns[0].Change)
		}
	}

	// Add emerging pattern history section
	if len(stats.EmergingPatternHistory) > 0 {
		report += "\n\nEmerging Pattern History:"
		
		// Loop through history in reverse to show most recent first
		for i := len(stats.EmergingPatternHistory) - 1; i >= 0; i-- {
			event := stats.EmergingPatternHistory[i]
			
			// Skip if the event has expired (more than 60 seconds old)
			if time.Since(event.StartTime) > 60*time.Second {
				continue
			}
			
			// Format time since the event
			timeSince := time.Since(event.StartTime).Seconds()
			report += fmt.Sprintf("\n• [%.0f sec ago] \"%s\" spiked %.0f%%", 
				timeSince, event.Pattern, event.PeakChange)
		}
	}

	// Add top errors
	if len(stats.ErrorCounts) > 0 {
		// Sort errors by count
		type errorEntry struct {
			Type  string
			Count int
		}
		
		var errors []errorEntry
		for errType, count := range stats.ErrorCounts {
			errors = append(errors, errorEntry{errType, count})
		}
		
		sort.Slice(errors, func(i, j int) bool {
			return errors[i].Count > errors[j].Count
		})
		
		report += "\n\n• Top Errors:"
		count := min(3, len(errors))
		for i := 0; i < count; i++ {
			report += fmt.Sprintf("\n  %d. %s (%s occurrences)",
				i+1, errors[i].Type, formatNumber(errors[i].Count))
		}
	}

	// Add alerts
	if len(d.alerts) > 0 {
		report += "\n\nSelf-Evolving Alerts:"
		
		// Get the most recent alerts (up to maxAlerts)
		start := len(d.alerts) - d.maxAlerts
		if start < 0 {
			start = 0
		}
		
		for i := start; i < len(d.alerts); i++ {
			alert := d.alerts[i]
			timestamp := alert.Timestamp.Format("15:04:05")
			report += fmt.Sprintf("\n[%s] %s", timestamp, alert.Message)
		}
	}

	// Add footer
	report += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\nPress Ctrl+C to exit\n"

	// Print the report
	fmt.Print(report)
}

// Helper functions
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}