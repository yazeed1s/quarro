// Package capability defines the small, capability-scoped interfaces a
// database engine package implements a subset of: Connector, Discoverer,
// SQLExecutor, Browser, Editor, Transactor, KeyValueStore, Aggregator,
// QueryCompiler, Explainer, and IdentifierFormatter. It imports no concrete
// engine package, so a caller, or a future engine package, can depend on
// exactly the capability it needs without pulling in every engine that
// exists.
package capability
