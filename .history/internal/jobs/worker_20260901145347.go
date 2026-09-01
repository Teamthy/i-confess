package jobs

import (
	"context"
	"log"
	"time"
)

// Worker manages background processing of queued jobs.
type Worker struct {
	q       *MemoryQueue
	workers int
	done    chan struct{}
	ticker  *time.Ticker
}

// NewWorker creates a new worker manager that will poll the queue at regular intervals.
func NewWorker(q *MemoryQueue, numWorkers int) *Worker {
	return &Worker{
		q:       q,
		workers: numWorkers,
		done:    make(chan struct{}),
		ticker:  time.NewTicker(1 * time.Second),
	}
}

// Start begins processing jobs from the queue in background goroutines.
// It returns immediately; call Stop() to gracefully shut down.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		go w.run(ctx, i)
	}
	log.Printf("queue: started %d worker goroutines", w.workers)
}

// Stop gracefully shuts down all worker goroutines.
func (w *Worker) Stop() {
	close(w.done)
	w.ticker.Stop()
}

func (w *Worker) run(ctx context.Context, id int) {
	for {
		select {
		case <-w.done:
			log.Printf("queue: worker %d stopping", id)
			return
		case <-w.ticker.C:
			if err := w.q.ProcessNext(ctx); err != nil {
				// Log non-handler-not-found errors; handler-not-found is normal if no jobs.
			}
		}
	}
}
