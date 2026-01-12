package log

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	// MaxLogFileSize is the maximum size of a log file before rotation (10MB)
	MaxLogFileSize = 10 * 1024 * 1024
)

var (
	logFile     *os.File
	logFilePath string
	logMutex    sync.Mutex
)

func init() {
	output := os.Stdout
	logrus.StandardLogger().SetFormatter(&logrus.TextFormatter{PadLevelText: true})
	logrus.StandardLogger().SetOutput(output)
}

// SetupFileLogging configures logging to write to a file with rotation
// If logPath is empty, logging continues to stdout
func SetupFileLogging(logPath string) error {
	if logPath == "" {
		return nil
	}

	logMutex.Lock()
	defer logMutex.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", dir, err)
	}

	// Open or create log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	// Close previous log file if exists
	if logFile != nil {
		_ = logFile.Close()
	}

	logFile = file
	logFilePath = logPath

	// Set logrus to use the file with a rotating writer
	logrus.StandardLogger().SetOutput(&rotatingWriter{})

	return nil
}

func PrintBuildInfo(ctx context.Context) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		G(ctx).Errorf("failed to get build info")
		return
	}
	var vcsRevision, vcsTime, vcsModified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			vcsRevision = setting.Value
		case "vcs.time":
			vcsTime = setting.Value
		case "vcs.modified":
			vcsModified = setting.Value
		}
	}

	G(ctx).WithFields(logrus.Fields{
		"revision": vcsRevision,
		"time":     vcsTime,
		"modified": vcsModified,
	}).Info("Build Information")
}

// rotatingWriter implements io.Writer with automatic rotation
type rotatingWriter struct{}

func (w *rotatingWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()

	// If log file is not set, write to stdout
	if logFile == nil {
		return os.Stdout.Write(p)
	}

	// Check if file still exists, recreate if deleted
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		// File was deleted, recreate it
		file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			// If we can't recreate, fall back to stdout
			logFile = nil
			return os.Stdout.Write(p)
		}
		if logFile != nil {
			_ = logFile.Close()
		}
		logFile = file
	}

	// Check file size and rotate if needed
	info, err := logFile.Stat()
	if err == nil && info.Size() >= MaxLogFileSize {
		// Truncate and start over
		_ = logFile.Close()
		file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			// If we can't truncate, fall back to stdout
			logFile = nil
			return os.Stdout.Write(p)
		}
		logFile = file
	}

	return logFile.Write(p)
}

func G(ctx context.Context) *logrus.Entry {
	return logrus.WithContext(ctx)
}
