package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Worker drains a Queue using a pool of goroutines.
//
// Each worker claims a job, runs it, and reports the outcome before claiming
// the next. Claiming is atomic in the queue, so N workers over one queue is
// safe; a job is handed to exactly one of them.
type Worker struct {
	q       Queue
	workers int
	// PollInterval is how long a worker waits after finding the queue empty.
	// Short enough that queued work starts promptly, long enough that an idle
	// server is not querying the database in a tight loop.
	PollInterval time.Duration
	// Name identifies this process in the worker_id column, so a job stuck in
	// running can be traced to the instance that took it.
	Name string

	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewWorker creates a worker pool over q.
func NewWorker(q Queue, numWorkers int) *Worker {
	if numWorkers < 1 {
		numWorkers = 1
	}
	return &Worker{
		q:            q,
		workers:      numWorkers,
		PollInterval: time.Second,
		Name:         "worker",
		done:         make(chan struct{}),
	}
}

// Start begins processing jobs in background goroutines. It returns
// immediately; call Stop to shut down.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.run(ctx, i)
	}
	log.Printf("queue: started %d worker goroutines", w.workers)
}

// Stop shuts the pool down and waits for in-flight jobs to finish.
//
// Safe to call more than once. Shutdown is commonly triggered from both a
// signal handler and a deferred call, and closing an already-closed channel
// panics - turning a clean shutdown into a crash.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() {
		close(w.done)
	})
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context, id int) {
	defer w.wg.Done()
	name := fmt.Sprintf("%s-%d", w.Name, id)
	poll := w.PollInterval
	if poll <= 0 {
		poll = time.Second
	}
	timer := time.NewTimer(poll)
	defer timer.Stop()

	for {
		select {
		case <-w.done:
			log.Printf("queue: worker %s stopping", name)
			return
		case <-ctx.Done():
			return
		default:
		}

		job, err := w.q.Claim(ctx, name)
		if err != nil {
			if err != ErrEmptyQueue && ctx.Err() == nil {
				log.Printf("queue: worker %s claim failed: %v", name, err)
			}
			// Nothing to do. Wait for the poll interval rather than spinning.
			timer.Reset(poll)
			select {
			case <-w.done:
				return
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			continue
		}

		// Something was available: drain without waiting for the next tick.
		w.execute(ctx, job)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
}

func (w *Worker) execute(ctx context.Context, job *Job) {
	h, ok := w.q.HandlerFor(job.Type)
	if !ok {
		// Not retryable: no amount of waiting registers the handler. Park it
		// with the reason attached so the queue view explains itself.
		reason := fmt.Sprintf("no handler registered for job type %q", job.Type)
		if err := w.q.FailPermanent(ctx, job.ID, reason); err != nil {
			log.Printf("queue: could not park job %s: %v", job.ID, err)
		}
		log.Printf("queue: %s", reason)
		return
	}

	err := h(ctx, job.Payload)
	if err == nil {
		if cerr := w.q.Complete(ctx, job.ID); cerr != nil {
			log.Printf("queue: job %s completed but could not be marked done: %v", job.ID, cerr)
		}
		return
	}

	if ferr := w.q.Fail(ctx, job.ID, err); ferr != nil {
		log.Printf("queue: job %s failed and could not be rescheduled: %v", job.ID, ferr)
		return
	}
	log.Printf("queue: job %s (%s) attempt %d failed: %v", job.ID, job.Type, job.Attempts, err)
}
