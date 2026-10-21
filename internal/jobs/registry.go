// Package jobs tracks cancellation, including a Stop arriving before Execute.
package jobs

import (
	"context"
	"sync"
	"time"
)

type job struct{ cancel context.CancelFunc }
type Registry struct {
	mu      sync.Mutex
	active  map[string]*job
	pending map[string]time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		active:  map[string]*job{},
		pending: map[string]time.Time{},
	}
}

func (r *Registry) StartScoped(
	id string,
	timeout time.Duration,
) (context.Context, func()) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	j := &job{cancel}
	r.mu.Lock()
	if old := r.active[id]; old != nil || len(r.active) >= 128 {
		cancel()
	} else {
		r.active[id] = j
	}
	if until, ok := r.pending[id]; ok {
		if time.Now().Before(until) {
			cancel()
		}
		delete(r.pending, id)
	}
	r.mu.Unlock()
	return ctx, func() {
		cancel()
		r.mu.Lock()
		if r.active[id] == j {
			delete(r.active, id)
		}
		r.mu.Unlock()
	}
}

func (r *Registry) Start(
	id string,
	timeout time.Duration,
) context.Context {
	ctx, _ := r.StartScoped(id, timeout)
	return ctx
}
func (r *Registry) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j := r.active[id]; j != nil {
		j.cancel()
		return true
	}
	now := time.Now()
	for key, until := range r.pending {
		if now.After(until) {
			delete(r.pending, key)
		}
	}
	if len(r.pending) < 256 {
		r.pending[id] = now.Add(30 * time.Second)
	}
	return false
}
func (r *Registry) Done(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j := r.active[id]; j != nil {
		j.cancel()
		delete(r.active, id)
	}
}
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, j := range r.active {
		j.cancel()
		delete(r.active, id)
	}
	clear(r.pending)
}

func (r *Registry) Running(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[id] != nil
}
