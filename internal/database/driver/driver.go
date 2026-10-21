// Package driver lets a database engine package register itself under a
// stable name, alongside metadata a frontend needs to render a connection
// form and picker entry without hardcoding that name. It imports
// internal/database/capability only, never a concrete engine package: an
// engine registers itself by calling Register from its own init(), so this
// package never has to know an engine package exists to route requests to
// it.
package driver

import "github.com/yazeed1s/quarro/internal/database/capability"

// ConfigField describes one field a driver's connection form needs, so the
// frontend can render it without a per-engine hardcoded form.
type ConfigField struct {
	Key      string `json:"key"` // "host", "port", "path", ...; the key under Profile.Config for a field Profile has no named field for
	Label    string `json:"label"`
	Kind     string `json:"kind"` // "text" | "port" | "password" | "filepath" | "bool"
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// DriverMetadata describes a driver for display and connection-form
// purposes.
type DriverMetadata struct {
	Name         string        `json:"name"`                 // stable identifier, e.g. "postgres"; matches connections.Engine's value for a built-in
	DisplayName  string        `json:"displayName"`          // "PostgreSQL"
	Icon         string        `json:"icon"`                 // icon key the frontend resolves to an asset
	SQLDialect   string        `json:"sqlDialect,omitempty"` // CodeMirror lang-sql dialect name; empty for a non-SQL engine
	ConfigSchema []ConfigField `json:"configSchema"`
}

// Driver is what an engine package registers: its identity, the
// capabilities it satisfies, and how to construct a live capability.Engine
// for a connection.
type Driver interface {
	Metadata() DriverMetadata
	Capabilities() capability.Capabilities
	New() capability.Engine
}
