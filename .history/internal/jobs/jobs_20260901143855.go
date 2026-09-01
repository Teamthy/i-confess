package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

type Handler func(ctx context.Context, payload map[string]any) error

type Job struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload,omitempty"`
	Status         string         `json:"status"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	RetryDelay     time.Duration  `json:"retry_delay,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastError      string         `json:"last_error,omitempty"`
}

type MemoryQueue struct {
	mu       sync.Mutex
	jobs     []Job
	handlers map[string]Handler
	seen     map[string]struct{}
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		jobs:     make([]Job, 0),
		handlers: make(map[string]Handler),
		seen:     make(map[string]struct{}),
	}
}

func (q *MemoryQueue) Register(name string, fn Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[name] = fn
}

func (q *MemoryQueue) Enqueue(job Job) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job.Type == "" {
		return "", errors.New("job type is required")
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 3
	}
	if job.RetryDelay == 0 {
		job.RetryDelay = 2 * time.Second
	}
	if job.Status == "" {
		job.Status = StatusQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	job.UpdatedAt = job.CreatedAt
	job.ID = job.ID
	if job.ID == "" {
		job.ID = randomID()
	}
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}
	if job.IdempotencyKey != "" {
		if _, exists := q.seen[job.IdempotencyKey]; exists {
			return "", fmt.Errorf("idempotency key already exists: %s", job.IdempotencyKey)
		}
		q.seen[job.IdempotencyKey] = struct{}{}
	}
	q.jobs = append(q.jobs, job)
	return job.ID, nil
}

func (q *MemoryQueue) ProcessNext(ctx context.Context) error {
	q.mu.Lock()
	if len(q.jobs) == 0 {
		q.mu.Unlock()
		return nil
	}
	idx := -1
	for i, job := range q.jobs {
		if job.Status == StatusQueued || job.Status == StatusFailed {
			idx = i
			break
		}
	}
	if idx == -1 {
		q.mu.Unlock()
		return nil
	}
	job := q.jobs[idx]
	q.mu.Unlock()

	handler, ok := q.handlers[job.Type]
	if !ok {
		q.mu.Lock()
		job.Status = StatusDeadLetter
		job.LastError = fmt.Sprintf("no handler registered for job type %q", job.Type)
		job.UpdatedAt = time.Now().UTC()
		q.jobs[idx] = job
		q.mu.Unlock()
		return errors.New(job.LastError)
	}

	job.Status = StatusRunning
	job.Attempts++
	job.UpdatedAt = time.Now().UTC()
	q.mu.Lock()
	q.jobs[idx] = job
	q.mu.Unlock()

	if err := handler(ctx, job.Payload); err != nil {
		q.mu.Lock()
		job.LastError = err.Error()
		job.UpdatedAt = time.Now().UTC()
		if job.Attempts >= job.MaxAttempts {
			job.Status = StatusDeadLetter
		} else {
			job.Status = StatusQueued
			if job.RetryDelay > 0 {
				job.UpdatedAt = time.Now().UTC().Add(job.RetryDelay)
			}
		}
		q.jobs[idx] = job
		q.mu.Unlock()
		return err
	}

	q.mu.Lock()
	job.Status = StatusCompleted
	job.LastError = ""
	job.UpdatedAt = time.Now().UTC()
	q.jobs[idx] = job
	q.mu.Unlock()
	return nil
}

func (q *MemoryQueue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, len(q.jobs))
	copy(out, q.jobs)
	return out
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
