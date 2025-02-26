// analyzer/patterns.go
// This file contains the implementation for tracking error patterns and their weights.

package analyzer

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"log_analyzer/models"
)

// ErrorPattern tracks statistics for an error pattern
type ErrorPattern struct {
	Count       int
	Weight      float64
	LastUpdated time.Time
	RateHistory []float64 // Stores rates for the last few time periods
}

// PatternTracker tracks error patterns and their weights
type PatternTracker struct {
	patterns       map[string]*ErrorPattern
	window         *SlidingWindow
	mux            sync.RWMutex
	historySize    int
	patternHistory []models.EmergingPatternEvent // Store pattern history here instead of in analyzer
}

// NewPatternTracker creates a new pattern tracker
func NewPatternTracker(window *SlidingWindow) *PatternTracker {
	return &PatternTracker{
		patterns:       make(map[string]*ErrorPattern),
		window:         window,
		historySize:    5, // Keep 5 time periods of history
		patternHistory: make([]models.EmergingPatternEvent, 0, 5), // Initialize history slice
	}
}

// UpdatePattern updates the statistics for an error pattern
func (pt *PatternTracker) UpdatePattern(entry models.LogEntry) {
	if entry.Level != "ERROR" || entry.ErrorType == "" {
		return
	}

	pt.mux.Lock()
	defer pt.mux.Unlock()

	pattern, exists := pt.patterns[entry.ErrorType]
	if !exists {
		pattern = &ErrorPattern{
			RateHistory: make([]float64, pt.historySize),
			LastUpdated: time.Now(),
		}
		pt.patterns[entry.ErrorType] = pattern
	}

	pattern.Count++
	now := time.Now()

	// Update rate history every 10 seconds
	if now.Sub(pattern.LastUpdated) > 10*time.Second {
		// Shift history and add new rate
		copy(pattern.RateHistory[1:], pattern.RateHistory[:pt.historySize-1])
		pattern.RateHistory[0] = pt.window.GetErrorRate(entry.ErrorType, 10)
		pattern.LastUpdated = now

		// Check for quadrupling of rate
		if len(pattern.RateHistory) >= 2 && 
           pattern.RateHistory[0] > 0 && 
           pattern.RateHistory[1] > 0 && 
           pattern.RateHistory[0] >= 4*pattern.RateHistory[1] {
			// Triple the weight if the rate quadrupled
			pattern.Weight = pattern.Weight*3.0
		}
	}
}

// WeightedError represents an error with its count and weight
type WeightedError struct {
	Type   string
	Count  int
	Weight float64
}

// GetTopErrors returns the top N errors by weighted count
func (pt *PatternTracker) GetTopErrors(n int) []WeightedError {
	pt.mux.RLock()
	defer pt.mux.RUnlock()

	if len(pt.patterns) == 0 {
		return []WeightedError{}
	}

	var result []WeightedError
	for errType, pattern := range pt.patterns {
		result = append(result, WeightedError{
			Type:   errType,
			Count:  pattern.Count,
			Weight: pattern.Weight,
		})
	}

	// Sort by weighted count (count * weight)
	sort.Slice(result, func(i, j int) bool {
		weightedI := float64(result[i].Count) * (1.0 + result[i].Weight)
		weightedJ := float64(result[j].Count) * (1.0 + result[j].Weight)
		return weightedI > weightedJ
	})

	if n > len(result) {
		n = len(result)
	}
	return result[:n]
}

// StoreEmergingPattern stores a significant pattern in history
func (pt *PatternTracker) StoreEmergingPattern(pattern string, change float64) {
	// Create a new event for the pattern history
	event := models.EmergingPatternEvent{
		Pattern:     pattern,
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(60 * time.Second), // Keep visible for 60 seconds
		PeakChange:  change,
		Description: fmt.Sprintf("Spike in %s errors", pattern),
	}
	
	// Use pattern tracker's own lock instead of analyzer's lock to avoid deadlock
	pt.mux.Lock()
	defer pt.mux.Unlock()
	
	// Store locally in the pattern tracker instead of in the analyzer
	pt.patternHistory = append(pt.patternHistory, event)
	
	// Keep only the last 5 events
	if len(pt.patternHistory) > 5 {
		pt.patternHistory = pt.patternHistory[1:]
	}
}

// GetPatternHistory returns the current pattern history
func (pt *PatternTracker) GetPatternHistory() []models.EmergingPatternEvent {
	pt.mux.RLock()
	defer pt.mux.RUnlock()
	
	// Return a copy to avoid concurrency issues
	result := make([]models.EmergingPatternEvent, len(pt.patternHistory))
	copy(result, pt.patternHistory)
	
	return result
}

// GetEmergingPatterns returns patterns with significant recent changes
func (pt *PatternTracker) GetEmergingPatterns() map[string]float64 {
	pt.mux.RLock()
	defer pt.mux.RUnlock()

	result := make(map[string]float64)
	significantPatterns := make([]string, 0)
	significantChanges := make([]float64, 0)
	
	// First collect all significant patterns without modifying anything
	for errType := range pt.patterns {
		// Calculate percentage change in the last 15 seconds compared to previous 15
		change := pt.window.GetErrorChange(errType, 15, 15)
		if change > 100.0 { // Only report significant increases (>100%)
			result[errType] = change
			significantPatterns = append(significantPatterns, errType)
			significantChanges = append(significantChanges, change)
		}
	}
	
	// Release read lock before calling StoreEmergingPattern
	pt.mux.RUnlock()
	
	// Now store each significant pattern separately
	for i, pattern := range significantPatterns {
		pt.StoreEmergingPattern(pattern, significantChanges[i])
	}
	
	// Re-acquire read lock for return
	pt.mux.RLock()
	
	return result
}
