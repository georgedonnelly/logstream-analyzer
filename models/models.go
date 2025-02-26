// models/models.go
// This file contains the data models used by the log analyzer.

package models

import (
	"sync"
	"time"
)

// LogEntry represents a parsed log entry
type LogEntry struct {
	Timestamp   time.Time
	Level       string
	IP          string
	Message     string
	ErrorType   string // For ERROR logs
	IsValid     bool   // Flag for valid parsing
	OriginalLog string // Original log string
}

// LogStats represents statistics for logs
type LogStats struct {
	EntriesProcessed  int
	CurrentRate       float64
	PeakRate          float64
	WindowSize        int // in seconds
	LevelCounts       map[string]int
	ErrorCounts       map[string]int
	ErrorRates        map[string]float64
	EmergingPatterns  map[string]float64 // pattern -> percentage increase
	SkippedEntries    int
	LastUpdated       time.Time
	mux               sync.RWMutex
	EmergingPatternHistory []EmergingPatternEvent
	PreviousWindowSize int // Track the previous window size for display
}

// EmergingPatternEvent tracks history of pattern spikes
type EmergingPatternEvent struct {
	Pattern     string
	StartTime   time.Time
	EndTime     time.Time
	PeakChange  float64
	Description string
}


// Alert represents an alert message
type Alert struct {
	Timestamp time.Time
	Message   string
}

// NewLogStats creates a new LogStats instance
func NewLogStats() *LogStats {
	stats := &LogStats{
			LevelCounts:      make(map[string]int),
			ErrorCounts:      make(map[string]int),
			ErrorRates:       make(map[string]float64),
			EmergingPatterns: make(map[string]float64),
			WindowSize:       60, // Default 60-second window
			PreviousWindowSize: 60, // Initialize same as starting window
			LastUpdated:      time.Now(),
			EmergingPatternHistory: make([]EmergingPatternEvent, 0, 5),
	}
	
	return stats
}


// Thread-safe methods for LogStats
func (s *LogStats) Lock()    { s.mux.Lock() }
func (s *LogStats) Unlock()  { s.mux.Unlock() }
func (s *LogStats) RLock()   { s.mux.RLock() }
func (s *LogStats) RUnlock() { s.mux.RUnlock() }
