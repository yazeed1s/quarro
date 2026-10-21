// Package sessions owns session handles and pinned per-document connections.
// It never imports database/sql: a session's database handle and a
// document's connection are opaque provider.Handle/provider.Connection
// values, opened, checked out, and closed only through the owning
// provider.Connector, this package just owns their lifecycle (which
// document has which connection, when to open/discard/close one), the same
// way it does for MongoDB's own connection resources.
//
// A session's provider is stored as provider.Engine (capability unknown):
// not every registered engine implements provider.Provider's full SQL
// surface (MongoDB doesn't), so this package only ever calls a capability it
// has type-asserted first: Connector for connection lifecycle (every
// engine has this) and Transactor for BEGIN/COMMIT/ROLLBACK (only SQL
// engines do; a MongoDB document's transaction calls fail with a clear
// error rather than a compile-time assumption that silently wouldn't have
// been possible to violate).
package sessions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/provider"
)

type documentHandle struct {
	conn   provider.Connection
	gate   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	tx     bool
}
type handle struct {
	provider  provider.Engine
	profile   connections.Profile
	db        provider.Handle
	profileID string
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	documents map[string]*documentHandle
}
type Registry struct {
	mu          sync.Mutex
	handles     map[string]*handle
	generations map[string]uint64
}

func NewRegistry() *Registry {
	return &Registry{
		handles:     map[string]*handle{},
		generations: map[string]uint64{},
	}
}

// connectorOf asserts that p supports connection lifecycle. Every engine
// registered with provider.Get does today; the check exists so a
// misconfigured or future capability-limited engine fails with a clear
// error here instead of a panic deep in this package.
func connectorOf(p provider.Engine) (provider.Connector, error) {
	c, ok := p.(provider.Connector)
	if !ok {
		return nil, fmt.Errorf(
			"this engine does not support opening connections",
		)
	}
	return c, nil
}

// transactorOf asserts that p supports BEGIN/COMMIT/ROLLBACK-style control
// statements. Only SQL engines do; MongoDB does not implement Transactor.
func transactorOf(p provider.Engine) (provider.Transactor, error) {
	t, ok := p.(provider.Transactor)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support transactions",
		)
	}
	return t, nil
}

// BeginConnect invalidates older connect attempts before any I/O starts.
func (r *Registry) BeginConnect(id string) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generations[id]++
	return r.generations[id]
}

func (r *Registry) Install(
	id, profile string,
	generation uint64,
	db provider.Handle,
) bool {
	p, _ := provider.Get(connections.EngineSQLite)
	return r.InstallProvider(
		id,
		connections.Profile{
			ID:     profile,
			Engine: connections.EngineSQLite,
		},
		generation,
		db,
		p,
	)
}

func (r *Registry) InstallProvider(
	id string,
	profile connections.Profile,
	generation uint64,
	db provider.Handle,
	p provider.Engine,
) bool {
	ctx, cancel := context.WithCancel(context.Background())
	h := &handle{
		db:        db,
		profileID: profile.ID,
		profile:   profile,
		provider:  p,
		ctx:       ctx,
		cancel:    cancel,
		documents: map[string]*documentHandle{},
	}
	r.mu.Lock()
	if r.generations[id] != generation {
		r.mu.Unlock()
		cancel()
		if connector, err := connectorOf(p); err == nil {
			_ = connector.Close(db)
		}
		return false
	}
	old := r.handles[id]
	r.handles[id] = h
	r.mu.Unlock()
	if old != nil {
		closeHandle(old)
	}
	return true
}
func (r *Registry) Put(id, profile string, db provider.Handle) {
	r.Install(id, profile, r.BeginConnect(id), db)
}
func (r *Registry) get(id string) (*handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.handles[id]
	if h == nil {
		return nil, fmt.Errorf("session %s is not connected", id)
	}
	return h, nil
}

func (r *Registry) Connected(
	id string,
) bool {
	_, err := r.get(id)
	return err == nil
}

// WithDocument serializes statements on the document's pinned connection.
// Disconnect cancels running work before waiting for connections to close.
func (r *Registry) WithDocument(
	ctx context.Context,
	id, document string,
	fn func(context.Context, provider.Connection) error,
) error {
	return r.WithExecution(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, _ provider.Engine) error { return fn(ctx, conn) },
	)
}

func (r *Registry) WithExecution(
	ctx context.Context,
	id, document string,
	fn func(context.Context, provider.Connection, provider.Engine) error,
) error {
	return r.withDocumentHandle(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, p provider.Engine, _ *documentHandle) error {
			return fn(ctx, conn, p)
		},
	)
}

func (r *Registry) withDocumentHandle(
	ctx context.Context,
	id, document string,
	fn func(context.Context, provider.Connection, provider.Engine, *documentHandle) error,
) error {
	h, err := r.get(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(h.ctx, cancel)
	defer stop()
	h.mu.Lock()
	if h.ctx.Err() != nil {
		h.mu.Unlock()
		return fmt.Errorf("session disconnected")
	}
	d := h.documents[document]
	if d == nil {
		if len(h.documents) >= 16 {
			h.mu.Unlock()
			return fmt.Errorf(
				"close an idle query tab before opening more execution contexts (limit 16)",
			)
		}
		connector, err := connectorOf(h.provider)
		if err != nil {
			h.mu.Unlock()
			return err
		}
		conn, err := connector.OpenConnection(ctx, h.db)
		if err != nil {
			h.mu.Unlock()
			return err
		}
		if h.profile.ReadOnly {
			if enforcer, ok := h.provider.(provider.ReadOnlyEnforcer); ok {
				if err := enforcer.EnforceReadOnly(ctx, conn); err != nil {
					h.mu.Unlock()
					_ = connector.DiscardConnection(conn)
					return fmt.Errorf("enforce read-only session: %w", err)
				}
			} else if _, sqlEngine := h.provider.(provider.SQLExecutor); sqlEngine {
				h.mu.Unlock()
				_ = connector.DiscardConnection(conn)
				return fmt.Errorf("this SQL engine cannot enforce read-only sessions")
			}
		}
		dctx, dcancel := context.WithCancel(h.ctx)
		d = &documentHandle{
			conn:   conn,
			gate:   make(chan struct{}, 1),
			ctx:    dctx,
			cancel: dcancel,
		}
		h.documents[document] = d
	}
	h.mu.Unlock()
	stopDocument := context.AfterFunc(d.ctx, cancel)
	defer stopDocument()
	select {
	case d.gate <- struct{}{}:
		defer func() { <-d.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx, d.conn, h.provider, d)
}

// WithAtomicEdits keeps a batch atomic without committing the user's transaction.
func (r *Registry) WithAtomicEdits(
	ctx context.Context,
	id, document string,
	fn func(context.Context, provider.Connection, provider.Engine) error,
) error {
	return r.withDocumentHandle(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, p provider.Engine, d *documentHandle) error {
			t, err := transactorOf(p)
			if err != nil {
				return err
			}
			begin, commit, rollback := "BEGIN", "COMMIT", "ROLLBACK"
			if d.tx {
				begin, commit, rollback = "SAVEPOINT quarro_edits", "RELEASE SAVEPOINT quarro_edits", "ROLLBACK TO SAVEPOINT quarro_edits"
			}
			if err := t.Exec(ctx, conn, begin); err != nil {
				return err
			}
			if err := fn(ctx, conn, p); err != nil {
				cleanup, cancel := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)
				defer cancel()
				if rollbackErr := t.Exec(cleanup, conn, rollback); rollbackErr != nil {
					return fmt.Errorf(
						"%w; rollback failed, outcome unknown: %v",
						err,
						rollbackErr,
					)
				}
				if d.tx {
					_ = t.Exec(cleanup, conn, commit)
				}
				return err
			}
			if err := t.Exec(ctx, conn, commit); err != nil {
				cleanup, cancel := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)
				defer cancel()
				_ = t.Exec(cleanup, conn, rollback)
				return fmt.Errorf(
					"batch completion failed; outcome unknown: %w",
					err,
				)
			}
			return nil
		},
	)
}

func (r *Registry) BeginDocumentTransaction(
	ctx context.Context,
	id, document string,
) error {
	return r.withDocumentHandle(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, p provider.Engine, d *documentHandle) error {
			if d.tx {
				return fmt.Errorf(
					"this document already has an active transaction",
				)
			}
			t, err := transactorOf(p)
			if err != nil {
				return err
			}
			if err := t.Exec(ctx, conn, "BEGIN"); err != nil {
				return err
			}
			d.tx = true
			return nil
		},
	)
}

func (r *Registry) CommitDocumentTransaction(
	ctx context.Context,
	id, document string,
) error {
	return r.withDocumentHandle(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, p provider.Engine, d *documentHandle) error {
			if !d.tx {
				return fmt.Errorf(
					"this document has no active transaction",
				)
			}
			t, err := transactorOf(p)
			if err != nil {
				return err
			}
			if err := t.Exec(ctx, conn, "COMMIT"); err != nil {
				return err
			}
			d.tx = false
			return nil
		},
	)
}

func (r *Registry) RollbackDocumentTransaction(
	ctx context.Context,
	id, document string,
) error {
	return r.withDocumentHandle(
		ctx,
		id,
		document,
		func(ctx context.Context, conn provider.Connection, p provider.Engine, d *documentHandle) error {
			if !d.tx {
				return fmt.Errorf(
					"this document has no active transaction",
				)
			}
			t, err := transactorOf(p)
			if err != nil {
				return err
			}
			if err := t.Exec(ctx, conn, "ROLLBACK"); err != nil {
				return err
			}
			d.tx = false
			return nil
		},
	)
}

func (r *Registry) DocumentTransactionActive(
	ctx context.Context,
	id, document string,
) (bool, error) {
	var active bool
	err := r.withDocumentHandle(
		ctx,
		id,
		document,
		func(_ context.Context, _ provider.Connection, _ provider.Engine, d *documentHandle) error {
			active = d.tx
			return nil
		},
	)
	return active, err
}
func discard(p provider.Engine, d *documentHandle) {
	if connector, err := connectorOf(p); err == nil {
		_ = connector.DiscardConnection(d.conn)
	}
}
func (r *Registry) CloseDocument(id, document string) {
	h, err := r.get(id)
	if err != nil {
		return
	}
	h.mu.Lock()
	d := h.documents[document]
	delete(h.documents, document)
	h.mu.Unlock()
	if d != nil {
		d.cancel()
		d.gate <- struct{}{}
		discard(h.provider, d)
		<-d.gate
	}
}
func closeHandle(h *handle) {
	h.cancel()
	h.mu.Lock()
	docs := h.documents
	h.documents = map[string]*documentHandle{}
	h.mu.Unlock()
	for _, d := range docs {
		d.gate <- struct{}{}
		discard(h.provider, d)
		<-d.gate
	}
	if connector, err := connectorOf(h.provider); err == nil {
		_ = connector.Close(h.db)
	}
}
func (r *Registry) Disconnect(id string) error {
	r.mu.Lock()
	r.generations[id]++
	h := r.handles[id]
	delete(r.handles, id)
	r.mu.Unlock()
	if h != nil {
		closeHandle(h)
	}
	return nil
}
func (r *Registry) CloseAll() {
	r.mu.Lock()
	handles := r.handles
	r.handles = map[string]*handle{}
	for id := range r.generations {
		r.generations[id]++
	}
	r.mu.Unlock()
	for _, h := range handles {
		closeHandle(h)
	}
}

func (r *Registry) Provider(id string) (provider.Engine, error) {
	h, e := r.get(id)
	if e != nil {
		return nil, e
	}
	return h.provider, nil
}
func (r *Registry) Profile(id string) (connections.Profile, error) {
	h, e := r.get(id)
	if e != nil {
		return connections.Profile{}, e
	}
	return h.profile, nil
}

func (r *Registry) Resources(
	id string,
) (provider.Handle, provider.Engine, error) {
	h, e := r.get(id)
	if e != nil {
		return nil, nil, e
	}
	return h.db, h.provider, nil
}
