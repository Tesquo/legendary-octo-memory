package processing

import (
	"context"
	"sync"
)

// jobRegistry tracks the work in flight for each media item.
//
// An item is keyed by its id, and one job for it by the token minted when that
// job reserved the item. Both are needed together: a job cancelled while it
// waits in the queue must be dropped when a worker finally reaches it, and a
// job that is finishing must not retire the reservation of the job that already
// replaced it. Keying on the media id alone (as the pipeline used to) lets those
// two jobs silently clobber each other's cancel entry.
type jobRegistry struct {
	mu      sync.Mutex
	next    uint64
	entries map[string]*jobEntry
}

// jobEntry is the reservation an item holds for one job at a time.
type jobEntry struct {
	token      uint64
	cancel     context.CancelFunc // nil while the job is still queued
	done       chan struct{}      // closed when the job stops; nil while queued
	superseded bool               // a caller asked for this cancel
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{entries: make(map[string]*jobEntry)}
}

// reserve claims an item for a new job and returns that job's token. It reports
// false when the item already has work in flight, so a duplicate submission can
// never run a second ffmpeg over the same output file.
func (r *jobRegistry) reserve(id string) (uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, busy := r.entries[id]; busy {
		return 0, false
	}

	r.next++
	e := &jobEntry{token: r.next}
	r.entries[id] = e

	return e.token, true
}

// claim attaches a starting job's cancel function to the reservation made for
// it. It reports false when the reservation is gone or belongs to another job:
// that is how a job cancelled or replaced while it waited in the queue is
// dropped instead of started. The job frees its slot again through release.
func (r *jobRegistry) claim(id string, token uint64, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok || e.token != token {
		return false
	}

	e.cancel = cancel
	e.done = make(chan struct{})

	return true
}

// release retires a job's reservation, freeing the item for a replacement and
// waking anyone waiting on the job. It ignores a job that no longer owns the
// entry, so a job finishing late cannot release someone else's slot.
func (r *jobRegistry) release(id string, token uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok || e.token != token {
		return
	}

	// Delete before closing: a waiter that wakes up must not find the item still
	// reserved, or the replacement job it is about to submit would be refused.
	delete(r.entries, id)
	if e.done != nil {
		close(e.done)
	}
}

// cancel stops the job in flight for an item and returns the channel to wait on
// for it to stop, or nil when there is nothing to wait for.
//
// A job that has not reached a worker yet is dropped outright — reservation and
// all — because starting it after the fact would recreate an output directory a
// delete already removed, or race the job that replaced it.
func (r *jobRegistry) cancel(id string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return nil
	}

	// The cancel came from a caller (delete, re-process) rather than from
	// shutdown: the caller decides what the item's outcome is, so the job itself
	// must not report one.
	e.superseded = true

	if e.cancel == nil {
		delete(r.entries, id)
		return nil
	}

	e.cancel()

	return e.done
}

// canceledByCaller reports whether this job's cancel came from cancel(). A
// shutdown cancels jobs through the pipeline's own context and is not a
// supersede: nobody is waiting to move the item on, so the job still reports.
func (r *jobRegistry) canceledByCaller(id string, token uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]

	return ok && e.token == token && e.superseded
}
