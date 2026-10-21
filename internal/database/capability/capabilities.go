package capability

// Capabilities is a record of what a driver can do. The interface-shaped
// fields are reflection-derived by Of, from the actual Go interfaces an
// Engine value satisfies; a caller checks c.Explainer instead of a
// string-keyed lookup or an engine-name comparison. The remaining fields
// describe quirks that do not map to a Go interface (there is no
// "SupportsAnalyze" method to reflect on); a driver's own Capabilities
// method (internal/database/driver.Driver) sets those explicitly, so Of
// leaves them at their zero value.
type Capabilities struct {
	Connector            bool `json:"connector"`
	Discoverer           bool `json:"discoverer"`
	SQLExecutor          bool `json:"sqlExecutor"`
	Browser              bool `json:"browser"`
	Editor               bool `json:"editor"`
	Transactor           bool `json:"transactor"`
	KeyValueStore        bool `json:"keyValueStore"`
	KeyValueEditor       bool `json:"keyValueEditor"`
	Aggregator           bool `json:"aggregator"`
	QueryCompiler        bool `json:"queryCompiler"`
	Explainer            bool `json:"explainer"`
	IdentifierFormatter  bool `json:"identifierFormatter"`
	RoutineDiscoverer    bool `json:"routineDiscoverer"`
	LazyDiscoverer       bool `json:"lazyDiscoverer"`
	ConstraintDiscoverer bool `json:"constraintDiscoverer"`
	DefinitionDiscoverer bool `json:"definitionDiscoverer"`
	DocumentEditor       bool `json:"documentEditor"`
	NativeQueryExecutor  bool `json:"nativeQueryExecutor"`

	// SupportsAnalyze reports whether Explainer.Explain's analyze flag
	// actually runs the statement instead of only estimating a plan.
	// False for SQLite, which has no execution-analysis mode.
	SupportsAnalyze bool `json:"supportsAnalyze"`
	// RequiresDelimiterWrapping reports whether a caller must wrap a
	// statement in a DELIMITER change before sending it for EXPLAIN or
	// execution, the way MySQL does for a stored routine body containing
	// its own semicolons.
	RequiresDelimiterWrapping bool `json:"requiresDelimiterWrapping"`
	// SupportsJoins reports whether Aggregator accepts a VisualQuery whose
	// Tables/joins span more than one object. False for engines with no
	// relational join concept (MongoDB, Redis).
	SupportsJoins bool `json:"supportsJoins"`
	// SupportsGroupBy reports whether Aggregator accepts a VisualQuery
	// with a GroupBy clause.
	SupportsGroupBy bool `json:"supportsGroupBy"`
	// NativeQueryLanguage names the language NativeQueryExecutor's req
	// carries, so the frontend picks the matching editor instead of
	// checking engine names: "mongodb-pipeline" or "redis-command". Empty
	// when the engine has no native query executor.
	NativeQueryLanguage string `json:"nativeQueryLanguage"`
}

// Of reflects over p and reports which capability interfaces it satisfies.
func Of(p Engine) Capabilities {
	return Capabilities{
		Connector:            implements[Connector](p),
		Discoverer:           implements[Discoverer](p),
		SQLExecutor:          implements[SQLExecutor](p),
		Browser:              implements[Browser](p),
		Editor:               implements[Editor](p),
		Transactor:           implements[Transactor](p),
		KeyValueStore:        implements[KeyValueStore](p),
		KeyValueEditor:       implements[KeyValueEditor](p),
		Aggregator:           implements[Aggregator](p),
		QueryCompiler:        implements[QueryCompiler](p),
		Explainer:            implements[Explainer](p),
		IdentifierFormatter:  implements[IdentifierFormatter](p),
		RoutineDiscoverer:    implements[RoutineDiscoverer](p),
		LazyDiscoverer:       implements[LazyDiscoverer](p),
		ConstraintDiscoverer: implements[ConstraintDiscoverer](p),
		DefinitionDiscoverer: implements[DefinitionDiscoverer](p),
		DocumentEditor:       implements[DocumentEditor](p),
		NativeQueryExecutor:  implements[NativeQueryExecutor](p),
	}
}

func implements[T any](p Engine) bool {
	_, ok := p.(T)
	return ok
}
