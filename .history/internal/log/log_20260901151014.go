package log

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

var currentLevel = LevelInfo

// Init sets up logging based on environment configuration.
func Init(level string) {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		currentLevel = level
	default:
		currentLevel = LevelInfo
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

// Debug logs a debug-level message.
func Debug(msg string, args ...any) {
	if shouldLog(LevelDebug) {
		log.Printf("[DEBUG] "+msg, args...)
	}
}

// Info logs an info-level message.
func Info(msg string, args ...any) {
	if shouldLog(LevelInfo) {
		log.Printf("[INFO] "+msg, args...)
	}
}

// Warn logs a warning-level message.
func Warn(msg string, args ...any) {
	if shouldLog(LevelWarn) {
		log.Printf("[WARN] "+msg, args...)
	}
}

// Error logs an error-level message.
func Error(msg string, args ...any) {
	if shouldLog(LevelError) {
		log.Printf("[ERROR] "+msg, args...)
	}
}

// Infof logs a formatted info message with context.
func Infof(msg string, keyvals ...any) {
	if shouldLog(LevelInfo) {
		formatted := msg
		for i := 0; i < len(keyvals); i += 2 {
			if i+1 < len(keyvals) {
				formatted += fmt.Sprintf(" %v=%v", keyvals[i], keyvals[i+1])
			}
		}
		log.Printf("[INFO] %s", formatted)
	}
}

// WithRequest logs a request with context.
func WithRequest(method, path, userID string, statusCode int, latencyMs int64) {
	Info("request completed method=%s path=%s user=%s status=%d latency_ms=%d",
		method, path, userID, statusCode, latencyMs)
}

// WithError logs an error with context.
func WithError(operation string, err error, context ...any) {
	msg := fmt.Sprintf("operation=%s error=%v", operation, err)
	for i := 0; i < len(context); i += 2 {
		if i+1 < len(context) {
			msg += fmt.Sprintf(" %v=%v", context[i], context[i+1])
		}
	}
	Error(msg)
}

// WithJob logs background job execution.
func WithJob(jobType, jobID, status string, latencyMs int64, err error) {
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	Info("job type=%s id=%s status=%s latency_ms=%d error=%s",
		jobType, jobID, status, latencyMs, errStr)
}

func shouldLog(level string) bool {
	levels := map[string]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return levels[level] >= levels[currentLevel]
}

// JSONLogger wraps structured logging (for future Datadog/ELK integration).
type JSONLogger struct {
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// LogJSON logs a structured message to stderr (for log aggregation pipelines).
func LogJSON(level, message, requestID, userID string, extra map[string]any) {
	entry := JSONLogger{
		Service:   "i-confess-backend",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   message,
		RequestID: requestID,
		UserID:    userID,
		Extra:     extra,
	}
	// In production, this should be marshaled to JSON and written to stderr
	// For now, just log the message
	if shouldLog(level) {
		fmt.Fprintf(os.Stderr, "%s [%s] %s (request_id=%s user=%s)\n",
			entry.Timestamp, entry.Level, entry.Message, entry.RequestID, entry.UserID)
	}
}
