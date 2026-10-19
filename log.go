package loch_queue

import (
	"io"
	"log"
	"os"
	"strings"
)

// setupLogOutput configures the log writer (stderr or file).
func setupLogOutput(config *Config) (io.Writer, *os.File) {
	var logOutput io.Writer = os.Stderr
	var fileHandle *os.File = nil // Keep track of file handle to close it later if possible
	if config.LogFile != "" {
		file, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf(logPrefix+"WARN: Could not open log file %s: %v. Defaulting to stderr.", config.LogFile, err)
		} else {
			logOutput = file
			fileHandle = file
		}
	}
	return logOutput, fileHandle
}

// logf provides leveled logging for the plugin.
func (qm *QueueManager) logf(level string, format string, v ...interface{}) {
	if qm.logger == nil {
		log.Printf(logPrefix+"[NO LOGGER] "+format, v...) // Fallback if logger somehow not initialized
		return
	}

	// Determine effective log level (debug flag overrides configured level)
	effectiveLogLevel := qm.config.LogLevel
	if qm.config.Debug {
		effectiveLogLevel = "debug"
	}

	// Check if the message should be logged based on its level and the effective log level
	shouldLog := false
	switch strings.ToLower(effectiveLogLevel) {
	case "debug":
		shouldLog = true // Debug logs everything
	case "info":
		shouldLog = level == "info" || level == "warn" || level == "error" || level == "debug" // Info also logs debug if debug is true
	case "warn":
		shouldLog = level == "warn" || level == "error"
	case "error":
		shouldLog = level == "error"
	default: // Unknown log level, default to info behavior
		shouldLog = level == "info" || level == "warn" || level == "error"
	}
	if qm.config.Debug && level != "debug" { // If debug is true, log everything above error as debug too
		if level == "info" || level == "warn" {
			// log as debug
		}
	}

	if shouldLog {
		qm.logger.Printf(logPrefix+strings.ToUpper(level)+": "+format, v...)
	}
}
