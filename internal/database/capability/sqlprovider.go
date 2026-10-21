package capability

// SQLProvider is the full capability set every SQL engine implements. It
// composes the smaller interfaces above rather than declaring its own
// method set, so a caller can depend on SQLProvider when it genuinely needs
// the whole SQL surface (as internal/sessions does) or on just the piece it
// needs otherwise.
type SQLProvider interface {
	Connector
	Discoverer
	SQLExecutor
	Browser
	Editor
	Transactor
	QueryCompiler
	Aggregator
	IdentifierFormatter
}
