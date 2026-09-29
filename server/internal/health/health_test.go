package health

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	_ "github.com/lib/pq"
)

func TestHandlerReportsDatabaseHealth(t *testing.T) {
	tests := []struct {
		name           string
		dsn            string
		wantHTTPStatus int
		wantStatus     string
		wantDBStatus   string
	}{
		{
			name:           "database reachable",
			dsn:            os.Getenv("TEST_DATABASE_URL"),
			wantHTTPStatus: http.StatusOK,
			wantStatus:     "healthy",
			wantDBStatus:   "healthy",
		},
		{
			name:           "database unavailable",
			dsn:            "host=127.0.0.1 port=1 user=iconfess dbname=postgres sslmode=disable connect_timeout=1",
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "degraded",
			wantDBStatus:   "unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dsn == "" {
				t.Skip("TEST_DATABASE_URL is required to exercise the healthy response")
			}
			sqlDB, err := sql.Open("postgres", tt.dsn)
			if err != nil {
				t.Fatalf("open database: %v", err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })

			recorder := httptest.NewRecorder()
			New(db.NewDB(sqlDB)).Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

			if recorder.Code != tt.wantHTTPStatus {
				t.Fatalf("HTTP status = %d, want %d; body: %s", recorder.Code, tt.wantHTTPStatus, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			var status HealthStatus
			if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
				t.Fatalf("decode health response: %v", err)
			}
			if status.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status.Status, tt.wantStatus)
			}
			if got := status.Checks["database"].Status; got != tt.wantDBStatus {
				t.Errorf("database status = %q, want %q", got, tt.wantDBStatus)
			}
			if tt.wantDBStatus == "unhealthy" && status.Checks["database"].Error == "" {
				t.Error("unhealthy database check omitted its error")
			}
		})
	}
}
