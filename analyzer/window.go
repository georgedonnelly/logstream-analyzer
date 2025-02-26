// analyzer/window.go
// This file contains the implementation for the sliding window used to track log entries.

package analyzer

import (
	"container/list"
	"sync"
	"time"

	"log_analyzer/models"
)

// SlidingWindow maintains a time-based window of log entries
type SlidingWindow struct {
	entries       *list.List
	entriesByType map[string]*list.List
	errorsByType  map[string]*list.List
	duration      time.Duration
	totalCount    int
	levelCounts   map[string]int
	errorCounts   map[string]int
	mux           sync.RWMutex
	analyzer      *Analyzer
}

// NewSlidingWindow creates a new sliding window with the specified duration
func NewSlidingWindow(durationSec int) *SlidingWindow {
	return &SlidingWindow{
		entries:       list.New(),
		entriesByType: make(map[string]*list.List),
		errorsByType:  make(map[string]*list.List),
		duration:      time.Duration(durationSec) * time.Second,
		levelCounts:   make(map[string]int),
		errorCounts:   make(map[string]int),
	}
}

// SetAnalyzer sets the analyzer reference
func (w *SlidingWindow) SetAnalyzer(analyzer *Analyzer) {
	w.analyzer = analyzer
}

// Add adds a log entry to the window
func (w *SlidingWindow) Add(entry models.LogEntry) {
	w.mux.Lock()
	defer w.mux.Unlock()

	now := time.Now()
	cutoff := now.Add(-w.duration)

	// Remove expired entries
	w.removeExpiredEntries(cutoff)

	// Add new entry
	w.entries.PushBack(entry)
	w.totalCount++

	// Update level counts
	w.levelCounts[entry.Level]++

	// Update type-specific lists
	if _, ok := w.entriesByType[entry.Level]; !ok {
		w.entriesByType[entry.Level] = list.New()
	}
	w.entriesByType[entry.Level].PushBack(entry)

	// Update error counts if applicable
	if entry.Level == "ERROR" && entry.ErrorType != "" {
		w.errorCounts[entry.ErrorType]++
		
		if _, ok := w.errorsByType[entry.ErrorType]; !ok {
			w.errorsByType[entry.ErrorType] = list.New()
		}
		w.errorsByType[entry.ErrorType].PushBack(entry)
	}
}

// SetDuration changes the window duration
func (w *SlidingWindow) SetDuration(durationSec int) {
	w.mux.Lock()
	defer w.mux.Unlock()

	oldDuration := w.duration
	w.duration = time.Duration(durationSec) * time.Second

	// If the window is shrinking, remove older entries
	if w.duration < oldDuration {
		w.removeExpiredEntries(time.Now().Add(-w.duration))
	}
}

// GetStats returns the current window statistics
func (w *SlidingWindow) GetStats() (int, map[string]int, map[string]int) {
	w.mux.RLock()
	defer w.mux.RUnlock()

	// Make copies of the maps
	levelCounts := make(map[string]int)
	errorCounts := make(map[string]int)

	for k, v := range w.levelCounts {
		levelCounts[k] = v
	}

	for k, v := range w.errorCounts {
		errorCounts[k] = v
	}

	return w.totalCount, levelCounts, errorCounts
}

// GetErrorRate calculates the rate of a specific error type over the last N seconds
func (w *SlidingWindow) GetErrorRate(errorType string, seconds int) float64 {
	w.mux.RLock()
	defer w.mux.RUnlock()

	if list, ok := w.errorsByType[errorType]; ok {
		cutoff := time.Now().Add(-time.Duration(seconds) * time.Second)
		count := 0

		for e := list.Back(); e != nil; e = e.Prev() {
			entry := e.Value.(models.LogEntry)
			if entry.Timestamp.Before(cutoff) {
				break
			}
			count++
		}

		return float64(count) / float64(seconds)
	}

	return 0
}

// GetErrorChange calculates the percentage change in error rate
func (w *SlidingWindow) GetErrorChange(errorType string, recentSec, prevSec int) float64 {
	w.mux.RLock()
	defer w.mux.RUnlock()

	if list, ok := w.errorsByType[errorType]; ok {
		now := time.Now()
		recentCutoff := now.Add(-time.Duration(recentSec) * time.Second)
		prevCutoff := recentCutoff.Add(-time.Duration(prevSec) * time.Second)
		
		recentCount := 0
		prevCount := 0

		for e := list.Back(); e != nil; e = e.Prev() {
			entry := e.Value.(models.LogEntry)
			if entry.Timestamp.After(recentCutoff) {
				recentCount++
			} else if entry.Timestamp.After(prevCutoff) {
				prevCount++
			} else {
				break
			}
		}

		// Calculate percentage change
		if prevCount == 0 {
			if recentCount > 0 {
				return 100.0 // 100% increase (from 0 to something)
			}
			return 0.0
		}

		return 100.0 * float64(recentCount-prevCount) / float64(prevCount)
	}

	return 0.0
}

// removeExpiredEntries removes entries older than the cutoff time
func (w *SlidingWindow) removeExpiredEntries(cutoff time.Time) {
	// Remove from main list and update counts
	for e := w.entries.Front(); e != nil; {
		entry := e.Value.(models.LogEntry)
		if entry.Timestamp.Before(cutoff) {
			next := e.Next()
			w.entries.Remove(e)
			w.totalCount--
			w.levelCounts[entry.Level]--
			
			// Remove from level-specific list
			if list, ok := w.entriesByType[entry.Level]; ok {
				for le := list.Front(); le != nil; {
					lEntry := le.Value.(models.LogEntry)
					if lEntry.Timestamp.Equal(entry.Timestamp) {
						nextLe := le.Next()
						list.Remove(le)
						le = nextLe
						break
					}
					le = le.Next()
				}
			}
			
			// Remove from error-specific list if applicable
			if entry.Level == "ERROR" && entry.ErrorType != "" {
				w.errorCounts[entry.ErrorType]--
				if list, ok := w.errorsByType[entry.ErrorType]; ok {
					for le := list.Front(); le != nil; {
						lEntry := le.Value.(models.LogEntry)
						if lEntry.Timestamp.Equal(entry.Timestamp) {
							nextLe := le.Next()
							list.Remove(le)
							le = nextLe
							break
						}
						le = le.Next()
					}
				}
			}
			
			e = next
		} else {
			break // Entries are sorted by time, so we can stop once we hit a non-expired entry
		}
	}
}
