// analyzer/analyzer.go
// Package analyzer provides a log analyzer that processes log entries and generates statistics
// It includes a deliberate concurrency bug that causes underreporting of error counts
// when processing high volumes of ERROR logs

package analyzer

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"log_analyzer/models"
)

// RateBucket tracks entries per second
type RateBucket struct {
	Count     int
	Timestamp time.Time
}

// Analyzer processes log entries and generates statistics
type Analyzer struct {
	window          *SlidingWindow
	patternTracker  *PatternTracker
	logChan         chan models.LogEntry
	statsChan       chan *models.LogStats
	alertChan       chan models.Alert
	stopChan        chan struct{}
	stats           *models.LogStats
	rateBuckets     []*RateBucket
	mux             sync.Mutex
	debugMode       bool
	debugLogger     *log.Logger
	skippedEntries  int
	bufferResized   bool
	bufferSize      int
	
	// For the deliberate concurrency bug
	buggyConcurrency bool
}

// NewAnalyzer creates a new Analyzer
func NewAnalyzer(
	logChan chan models.LogEntry,
	statsChan chan *models.LogStats,
	alertChan chan models.Alert,
	debugMode bool,
	initialBufferSize int,
) *Analyzer {
	a := &Analyzer{
		window:         NewSlidingWindow(60), // Start with 60-second window
		logChan:        logChan,
		statsChan:      statsChan,
		alertChan:      alertChan,
		stopChan:       make(chan struct{}),
		stats:          models.NewLogStats(),
		rateBuckets:    make([]*RateBucket, 0, 120), // Track up to 120 seconds
		debugMode:      debugMode,
		bufferSize:     initialBufferSize, // Initial buffer size
		
		// Deliberately adding the concurrency bug flag
		buggyConcurrency: true,
	}

	a.window.SetAnalyzer(a)

	a.patternTracker = NewPatternTracker(a.window)

	if debugMode {
		f, err := os.OpenFile("debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Failed to create debug log file: %v", err)
		}
		a.debugLogger = log.New(f, "ANALYZER: ", log.LstdFlags)
	}

	return a
}

// Start begins analyzing logs
func (a *Analyzer) Start() {
	go a.processLogs()
	go a.updateStats()
}

// Stop signals the analyzer to stop
func (a *Analyzer) Stop() {
	close(a.stopChan)
}

func (a *Analyzer) processLogs() {
	secondBucket := time.Now().Truncate(time.Second)
	secondCount := 0

	for {
		select {
		case <-a.stopChan:
			return
		case entry := <-a.logChan:
			now := time.Now()

			// Check if we need to update rate bucket
			if now.Truncate(time.Second) != secondBucket {
				a.updateRateBucket(secondBucket, secondCount)
				secondBucket = now.Truncate(time.Second)
				secondCount = 0
			}
			secondCount++

			if !entry.IsValid {
				a.mux.Lock()
				a.skippedEntries++
				a.mux.Unlock()
				continue
			}

			// Process the entry
			a.window.Add(entry)
			a.patternTracker.UpdatePattern(entry)

			// Here's where we implement the deliberate concurrency bug
            // The bug will cause error counts to be underreported when processing
            // high volumes of ERROR logs
            if entry.Level == "ERROR" && a.buggyConcurrency {
                // Bug: No lock protection when processing ERROR logs at high rates
                // This will cause race conditions when error rate is high
                a.stats.EntriesProcessed++ // Missing lock, only triggered for ERROR logs
            } else {
                a.mux.Lock()
                a.stats.EntriesProcessed++
                a.mux.Unlock()
            }

			// Check for buffer resize need
			if secondCount > int(float64(a.bufferSize) * 0.8) {
				newSize := int(float64(a.bufferSize) * 1.5)
				a.mux.Lock()
				a.bufferSize = newSize
				a.bufferResized = true
				a.mux.Unlock()
				
				// Send alert about buffer resize
				a.alertChan <- models.Alert{
					Timestamp: now,
					Message:   fmt.Sprintf("⚠️ Burst detected: %d entries in 1 sec, resized buffer to %d", secondCount, newSize),
				}
				
				if a.debugMode {
					a.debugLogger.Printf("Resized buffer to %d due to high load", newSize)
				}
			}
		}
	}
}

func (a *Analyzer) updateRateBucket(timestamp time.Time, count int) {
	a.mux.Lock()
	defer a.mux.Unlock()

	// Add new bucket
	a.rateBuckets = append(a.rateBuckets, &RateBucket{
		Count:     count,
		Timestamp: timestamp,
	})

	// Remove buckets older than 120 seconds (our max window size)
	cutoff := time.Now().Add(-120 * time.Second)
	newBuckets := make([]*RateBucket, 0, len(a.rateBuckets))
	for _, bucket := range a.rateBuckets {
		if bucket.Timestamp.After(cutoff) {
			newBuckets = append(newBuckets, bucket)
		}
	}
	a.rateBuckets = newBuckets
}

func (a *Analyzer) updateStats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
			stats := a.generateStats()
			a.statsChan <- stats
		}
	}
}

func (a *Analyzer) generateStats() *models.LogStats {
	a.mux.Lock()
	defer a.mux.Unlock()

	// Calculate current processing rate
	currentRate := a.calculateRate(10) // Last 10 seconds
	
	// Update peak rate if needed
	if currentRate > a.stats.PeakRate {
		a.stats.PeakRate = currentRate
	}

	// Adjust window size based on processing rate
	newWindowSize := a.stats.WindowSize
	if currentRate > 2500 && a.stats.WindowSize > 30 {
		newWindowSize = max(30, a.stats.WindowSize-10)
		a.alertChan <- models.Alert{
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("⚠️ Adjusted window to %d sec due to rate surge", newWindowSize),
		}
	} else if currentRate < 600 && a.stats.WindowSize < 120 {
		newWindowSize = min(120, a.stats.WindowSize+10)
	}

	// If window size changed, update it
	if newWindowSize != a.stats.WindowSize {
		a.stats.WindowSize = newWindowSize
		a.window.SetDuration(newWindowSize)
		
		if a.debugMode {
			a.debugLogger.Printf("Adjusted window size to %d seconds based on rate: %.2f entries/sec", 
				newWindowSize, currentRate)
		}
	}

	// Get current window statistics
	_, levelCounts, errorCounts := a.window.GetStats()

	// Update stats
	a.stats.CurrentRate = currentRate
	a.stats.LevelCounts = levelCounts
	a.stats.ErrorCounts = errorCounts
	a.stats.LastUpdated = time.Now()
	a.stats.SkippedEntries = a.skippedEntries

	// Get error rates
	a.stats.ErrorRates = make(map[string]float64)
	for errType := range errorCounts {
		a.stats.ErrorRates[errType] = a.window.GetErrorRate(errType, a.stats.WindowSize)
	}

	// Get emerging patterns
	a.stats.EmergingPatterns = a.patternTracker.GetEmergingPatterns()

	// Get pattern history
	a.stats.EmergingPatternHistory = a.patternTracker.GetPatternHistory()

	// Check if we need to send an alert for high error rate
	totalErrorRate := 0.0
	for _, rate := range a.stats.ErrorRates {
		totalErrorRate += rate
	}

	if totalErrorRate > 5.0 {
		a.alertChan <- models.Alert{
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("⚠️ High error rate (%.1f errors/sec), increased pattern weight", totalErrorRate),
		}
	}

	// Reset buffer resize flag after reporting it once
	if a.bufferResized {
		a.bufferResized = false
	}

	// Return a copy to avoid concurrency issues
	return a.cloneStats()
}

func (a *Analyzer) calculateRate(seconds int) float64 {
	now := time.Now()
	cutoff := now.Add(-time.Duration(seconds) * time.Second)
	
	var totalCount int
	var relevantBuckets int
	
	for _, bucket := range a.rateBuckets {
		if bucket.Timestamp.After(cutoff) {
			totalCount += bucket.Count
			relevantBuckets++
		}
	}
	
	if relevantBuckets == 0 {
		return 0.0
	}
	
	return float64(totalCount) / float64(relevantBuckets)
}

func (a *Analyzer) cloneStats() *models.LogStats {
	clone := models.NewLogStats()
	
	clone.EntriesProcessed = a.stats.EntriesProcessed
	clone.CurrentRate = a.stats.CurrentRate
	clone.PeakRate = a.stats.PeakRate
	clone.WindowSize = a.stats.WindowSize
	clone.LastUpdated = a.stats.LastUpdated
	clone.SkippedEntries = a.stats.SkippedEntries
	
	// Copy maps
	for k, v := range a.stats.LevelCounts {
		clone.LevelCounts[k] = v
	}
	
	for k, v := range a.stats.ErrorCounts {
		clone.ErrorCounts[k] = v
	}
	
	for k, v := range a.stats.ErrorRates {
		clone.ErrorRates[k] = v
	}
	
	for k, v := range a.stats.EmergingPatterns {
		clone.EmergingPatterns[k] = v
	}

	clone.EmergingPatternHistory = make([]models.EmergingPatternEvent, 
		len(a.stats.EmergingPatternHistory))
	copy(clone.EmergingPatternHistory, a.stats.EmergingPatternHistory)
	
	return clone
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
