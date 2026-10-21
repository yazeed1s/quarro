package capability

// IdentifierFormatter exposes this engine's own quoting and literal-encoding
// rules (already used internally for edits and visual queries) so a SQL
// export can generate dialect-correct INSERT text without a second,
// divergent quoting implementation in internal/app.
type IdentifierFormatter interface {
	QuoteIdentifier(string) string
	FormatLiteral(any) (string, error)
	// QualifiedTable resolves an object identifier (a bare table name for
	// SQLite, or the schema/table JSON pair used elsewhere in this app for
	// PostgreSQL/MySQL) into properly quoted SQL text for that engine, e.g.
	// for a SQL-dump export's INSERT INTO target.
	QualifiedTable(string) (string, error)
}
