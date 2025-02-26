// reader/reader.go - Reads log entries from stdin and sends them to a channel for processing.


package reader

import (
	"bufio"
	"log"
	"os"
	"regexp"
	"time"

	"log_analyzer/models"
)

var (
	logRegex   = regexp.MustCompile(`\[(.*?)\] (ERROR|INFO|DEBUG) - IP:([\d\.]+)(?: (.*))?`)
	errorRegex = regexp.MustCompile(`Error 500 - (.*)`)
)

// Reader reads log entries from stdin
type Reader struct {
	logChan     chan models.LogEntry
	stopChan    chan struct{}
	debugMode   bool
	debugLogger *log.Logger
}

// NewReader creates a new Reader
func NewReader(logChan chan models.LogEntry, debugMode bool) *Reader {
	r := &Reader{
		logChan:   logChan,
		stopChan:  make(chan struct{}),
		debugMode: debugMode,
	}
	
	if debugMode {
		f, err := os.OpenFile("debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Failed to create debug log file: %v", err)
		}
		r.debugLogger = log.New(f, "READER: ", log.LstdFlags)
	}
	
	return r
}

// Start begins reading from stdin
func (r *Reader) Start() {
	go r.readLogs()
}

// Stop signals the reader to stop
func (r *Reader) Stop() {
	close(r.stopChan)
}

func (r *Reader) readLogs() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // Larger buffer for high volume

	for scanner.Scan() {
		select {
		case <-r.stopChan:
			return
		default:
			logText := scanner.Text()
			entry := r.parseLine(logText)
			if r.debugMode && !entry.IsValid {
				r.debugLogger.Printf("Skipped malformed entry: %s", logText)
			}
			r.logChan <- entry
		}
	}

	if err := scanner.Err(); err != nil {
		if r.debugMode {
			r.debugLogger.Printf("Scanner error: %v", err)
		}
		log.Printf("Error reading stdin: %v", err)
	}
}

func (r *Reader) parseLine(line string) models.LogEntry {
	entry := models.LogEntry{
		OriginalLog: line,
		IsValid:     false,
	}

	// Handle empty lines and completely malformed entries gracefully
	if line == "" {
		return entry
	}

	matches := logRegex.FindStringSubmatch(line)
	if matches == nil || len(matches) < 4 {
		return entry
	}

	// Parse timestamp
	timestamp, err := time.Parse("2006-01-02T15:04:05Z", matches[1])
	if err != nil {
		return entry
	}

	entry.Timestamp = timestamp
	entry.Level = matches[2]
	entry.IP = matches[3]
	entry.IsValid = true

	// Parse error message if present
	if entry.Level == "ERROR" && len(matches) > 4 && matches[4] != "" {
		entry.Message = matches[4]
		errorMatches := errorRegex.FindStringSubmatch(matches[4])
		if errorMatches != nil && len(errorMatches) > 1 {
			entry.ErrorType = errorMatches[1]
		}
	}

	return entry
}
