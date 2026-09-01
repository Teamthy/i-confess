package health

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// Checker performs application health checks.
type Checker struct {
	db *sql.DB
}

func New(db *sql.DB) *Checker {
	return &Checker{db: db}
}

// Check performs all health checks and returns the result.
func (c *Checker) Check(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    make(map[string]CheckResult),
	}

	// Database check
	if err := c.checkDatabase(ctx); err != nil {
		status.Checks["database"] = CheckResult{Status: "unhealthy", Error: err.Error()}
		status.Status = "degraded"
	} else {
		status.Checks["database"] = CheckResult{Status: "healthy"}
	}

	return status
}

// Handler returns an HTTP handler for health checks.
func (c *Checker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status := c.Check(ctx)

		// Return 503 if not healthy
		statusCode := http.StatusOK
		if status.Status != "healthy" {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = httpx.EncodeJSON(w, status)
	}
}

func (c *Checker) checkDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.db.PingContext(ctx)
}

// HealthStatus represents the application health status.
type HealthStatus struct {
	Status    string                    `json:"status"` // healthy | degraded | unhealthy
	Timestamp string                    `json:"timestamp"`
	Checks    map[string]CheckResult    `json:"checks,omitempty"`
}

// CheckResult represents an individual health check result.
type CheckResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
