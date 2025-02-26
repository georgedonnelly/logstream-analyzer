package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"log_analyzer/analyzer"
	"log_analyzer/display"
	"log_analyzer/models"
	"log_analyzer/reader"
)

const (
	LogChannelSize   = 50000
	StatsChannelSize = 10
	AlertChannelSize = 100
)

func main() {

	// Start with smaller buffer size in order to test buffer resize events more thoroughly
	bufferSize := flag.Int("buffer", 10000, "Initial buffer size for log entries")

	// Parse command-line flags
	debugMode := flag.Bool("debug", false, "Enable debug mode with detailed logging")
	flag.Parse()

	// Create channels for communication between components
	logChan := make(chan models.LogEntry, LogChannelSize)
	statsChan := make(chan *models.LogStats, StatsChannelSize)
	alertChan := make(chan models.Alert, AlertChannelSize)

	// Create components
	logReader := reader.NewReader(logChan, *debugMode)
	logAnalyzer := analyzer.NewAnalyzer(logChan, statsChan, alertChan, *debugMode, *bufferSize)
	logDisplay := display.NewDisplay(statsChan, alertChan)

	// Start components
	logReader.Start()
	logAnalyzer.Start()
	logDisplay.Start()

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down gracefully...")

	// Stop components in reverse order
	logDisplay.Stop()
	logAnalyzer.Stop()
	logReader.Stop()

	fmt.Println("Shutdown complete.")
}
