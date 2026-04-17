package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	infoLogger  *log.Logger
	errorLogger *log.Logger
	logDir      string
	logFile     *os.File
	errorFile   *os.File
	currentDate string
	mu          sync.Mutex
)

// Init initializes the logger with log directory
func Init() error {
	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Create log directory
	logDir = filepath.Join(exeDir, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log files for today
	if err := rotateLogFiles(); err != nil {
		return err
	}

	return nil
}

// rotateLogFiles creates or rotates log files based on current date
func rotateLogFiles() error {
	mu.Lock()
	defer mu.Unlock()

	today := time.Now().Format("2006-01-02")

	// Check if we need to rotate
	if currentDate == today && logFile != nil && errorFile != nil {
		return nil
	}

	// Close existing files
	if logFile != nil {
		logFile.Close()
	}
	if errorFile != nil {
		errorFile.Close()
	}

	// Open new log files
	logPath := filepath.Join(logDir, fmt.Sprintf("outview-%s.log", today))
	errorPath := filepath.Join(logDir, fmt.Sprintf("outview-error-%s.log", today))

	var err error
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	errorFile, err = os.OpenFile(errorPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logFile.Close()
		return fmt.Errorf("failed to open error log file: %w", err)
	}

	// Create multi-writers (console + file)
	infoWriter := io.MultiWriter(os.Stdout, logFile)
	errorWriter := io.MultiWriter(os.Stderr, errorFile)

	// Initialize loggers
	infoLogger = log.New(infoWriter, "", log.LstdFlags)
	errorLogger = log.New(errorWriter, "[ERROR] ", log.LstdFlags|log.Lshortfile)

	currentDate = today
	return nil
}

// checkRotation checks if log files need rotation (called periodically)
func checkRotation() {
	if err := rotateLogFiles(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate log files: %v\n", err)
	}
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	checkRotation()
	if infoLogger != nil {
		infoLogger.Printf(format, v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	checkRotation()
	if errorLogger != nil {
		errorLogger.Printf(format, v...)
	} else {
		fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", v...)
	}
}

// Debug logs a debug message (same as Info for now)
func Debug(format string, v ...interface{}) {
	checkRotation()
	if infoLogger != nil {
		infoLogger.Printf("[Debug] "+format, v...)
	} else {
		fmt.Printf("[Debug] "+format+"\n", v...)
	}
}

// Close closes log files
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	if errorFile != nil {
		errorFile.Close()
		errorFile = nil
	}
}
