package capability

// Handle is an opaque, provider-owned database handle. internal/sessions
// stores and passes it around without knowing its concrete type; only the
// provider that created it (via Connector.Open) ever type-asserts it back
// to a concrete type: *sql.DB for every SQL engine today. A future
// document or key-value engine's Handle would wrap its own client type
// instead.
type Handle = any

// Connection is an opaque, provider-owned single connection resource
// checked out from a Handle (via Connector.OpenConnection): the unit of
// work every other capability below operates on. Only the owning provider
// type-asserts it back to a concrete type: *sql.Conn for every SQL engine
// today.
type Connection = any

// Engine is an opaque provider value of unknown capability: what a driver
// constructs and what internal/sessions stores and threads through,
// without assuming any engine supports any particular capability. A caller
// that needs a specific one type-asserts it to that capability's
// interface, or checks Of(p) first.
type Engine = any
