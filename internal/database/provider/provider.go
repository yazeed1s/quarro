// Package provider re-exports internal/database/capability's interfaces
// under the names existing call sites (internal/app, internal/sessions)
// already use, and routes a connection profile's engine to its concrete
// implementation via internal/database/driver's registry. This package
// itself imports no concrete engine package: each engine registers itself
// with that registry from its own init(), triggered by whatever imports
// the engine package by name (main.go, for the real application; a test
// package's own blank import, for a test binary built in isolation).
package provider

import (
	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

type (
	// Handle is an opaque, provider-owned database handle. See
	// capability.Handle for the full doc comment.
	Handle = capability.Handle
	// Connection is an opaque, provider-owned single connection resource.
	// See capability.Connection for the full doc comment.
	Connection = capability.Connection
	// Engine is an opaque provider value of unknown capability. See
	// capability.Engine for the full doc comment.
	Engine = capability.Engine

	Connector            = capability.Connector
	Discoverer           = capability.Discoverer
	SQLExecutor          = capability.SQLExecutor
	Browser              = capability.Browser
	Editor               = capability.Editor
	Transactor           = capability.Transactor
	KeyValueStore        = capability.KeyValueStore
	KeyValueEditor       = capability.KeyValueEditor
	Aggregator           = capability.Aggregator
	QueryCompiler        = capability.QueryCompiler
	Explainer            = capability.Explainer
	IdentifierFormatter  = capability.IdentifierFormatter
	RoutineDiscoverer    = capability.RoutineDiscoverer
	LazyDiscoverer       = capability.LazyDiscoverer
	ConstraintDiscoverer = capability.ConstraintDiscoverer
	DefinitionDiscoverer = capability.DefinitionDiscoverer
	DocumentEditor       = capability.DocumentEditor
	NativeQueryExecutor  = capability.NativeQueryExecutor
	NativeWriteDetector  = capability.NativeWriteDetector
	ReadOnlyEnforcer     = capability.ReadOnlyEnforcer

	// SQLProvider is the full capability set every engine in this package
	// implements today. See capability.SQLProvider for the full doc comment.
	SQLProvider = capability.SQLProvider
)

// Provider is an alias for SQLProvider, kept for call sites that only ever
// deal with SQL engines and want the full surface by name (most of
// internal/app's query/export/import services). New code that only needs
// one capability should depend on that capability's interface directly.
type Provider = SQLProvider

// Get constructs the engine registered under this profile's engine name.
func Get(engine connections.Engine) (Engine, error) {
	d, err := driver.Get(string(engine))
	if err != nil {
		return nil, err
	}
	return d.New(), nil
}
