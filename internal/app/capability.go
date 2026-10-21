package app

import (
	"fmt"

	"github.com/yazeed1s/quarro/internal/database/provider"
)

// requireSQL asserts that a session's provider supports the full SQL
// surface (query execution, browsing, editing, compilation) that
// QueriesService, ExportService, and ImportService assume. A session
// connected to a non-SQL engine (MongoDB) returns a clear error here
// instead of a confusing low-level type-assertion panic, the backend
// enforcing capabilities rather than relying on the frontend not to ask.
func requireSQL(p provider.Engine) (provider.SQLProvider, error) {
	sqlP, ok := p.(provider.SQLProvider)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support SQL execution",
		)
	}
	return sqlP, nil
}

// requireConnector asserts that p supports opening/closing a database
// handle, which every registered engine does today, SQL or not.
func requireConnector(p provider.Engine) (provider.Connector, error) {
	c, ok := p.(provider.Connector)
	if !ok {
		return nil, fmt.Errorf(
			"this engine does not support opening connections",
		)
	}
	return c, nil
}

// requireDiscoverer and requireBrowser back QueriesService methods that
// work the same for MongoDB as for a SQL engine (listing
// databases/collections, paging a collection/table), unlike requireSQL,
// which is only ever satisfied by a SQL engine.
func requireDiscoverer(
	p provider.Engine,
) (provider.Discoverer, error) {
	d, ok := p.(provider.Discoverer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support schema discovery",
		)
	}
	return d, nil
}

func requireLazyDiscoverer(
	p provider.Engine,
) (provider.LazyDiscoverer, error) {
	d, ok := p.(provider.LazyDiscoverer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support lazy schema discovery",
		)
	}
	return d, nil
}
func requireBrowser(p provider.Engine) (provider.Browser, error) {
	b, ok := p.(provider.Browser)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support browsing",
		)
	}
	return b, nil
}

// requireKeyValue backs QueriesService's key-value methods (Redis): a SQL
// engine or MongoDB session hitting these gets this same clear error, not
// a panic.
func requireKeyValue(
	p provider.Engine,
) (provider.KeyValueStore, error) {
	kv, ok := p.(provider.KeyValueStore)
	if !ok {
		return nil, fmt.Errorf(
			"this connection is not a key-value store",
		)
	}
	return kv, nil
}

// requireKeyValueEditor backs QueriesService.ApplyKeyEdit: only Redis
// writes keys; any other session gets this error, not a panic.
func requireKeyValueEditor(
	p provider.Engine,
) (provider.KeyValueEditor, error) {
	kv, ok := p.(provider.KeyValueEditor)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support editing keys",
		)
	}
	return kv, nil
}

// requireAggregator asserts that p can run a Visual Query AST directly. SQL
// engines satisfy this through the shared dialect compile-and-execute helper;
// future document engines may satisfy it without supporting raw SQL.
func requireAggregator(
	p provider.Engine,
) (provider.Aggregator, error) {
	agg, ok := p.(provider.Aggregator)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support visual query aggregation",
		)
	}
	return agg, nil
}

// requireExplainer asserts that p can run EXPLAIN/EXPLAIN ANALYZE. Only
// PostgreSQL implements provider.Explainer so far; a session connected to
// any other engine gets this clear error instead of a type-assertion
// panic.
func requireExplainer(p provider.Engine) (provider.Explainer, error) {
	exp, ok := p.(provider.Explainer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support EXPLAIN",
		)
	}
	return exp, nil
}

// requireRoutineDiscoverer asserts that p can list stored functions/
// procedures. Only PostgreSQL and MySQL implement provider.RoutineDiscoverer;
// any other engine gets this clear error instead of a type-assertion panic.
func requireRoutineDiscoverer(
	p provider.Engine,
) (provider.RoutineDiscoverer, error) {
	rd, ok := p.(provider.RoutineDiscoverer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support routine discovery",
		)
	}
	return rd, nil
}

// requireConstraintDiscoverer asserts that p can list named CHECK/UNIQUE
// constraints. Only PostgreSQL and MySQL implement
// provider.ConstraintDiscoverer.
func requireConstraintDiscoverer(
	p provider.Engine,
) (provider.ConstraintDiscoverer, error) {
	cd, ok := p.(provider.ConstraintDiscoverer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support constraint discovery",
		)
	}
	return cd, nil
}

// requireDefinitionDiscoverer asserts that p can return an object's
// engine-native DDL text. SQLite, PostgreSQL, and MySQL all implement
// provider.DefinitionDiscoverer.
func requireDefinitionDiscoverer(
	p provider.Engine,
) (provider.DefinitionDiscoverer, error) {
	dd, ok := p.(provider.DefinitionDiscoverer)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support object definitions",
		)
	}
	return dd, nil
}

// requireDocumentEditor asserts that p can insert/replace/delete a whole
// document by _id. Only MongoDB implements provider.DocumentEditor; a SQL
// engine's own row editing goes through requireSQL/Editor instead, since a
// flat column grid and a whole-document JSON edit are genuinely different
// capabilities, not two names for the same one.
func requireDocumentEditor(
	p provider.Engine,
) (provider.DocumentEditor, error) {
	de, ok := p.(provider.DocumentEditor)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support document editing",
		)
	}
	return de, nil
}

// requireNativeQueryExecutor asserts that p can run one engine-native
// query directly. Only MongoDB implements provider.NativeQueryExecutor; a
// SQL engine's own query documents go through requireSQL instead.
func requireNativeQueryExecutor(
	p provider.Engine,
) (provider.NativeQueryExecutor, error) {
	nq, ok := p.(provider.NativeQueryExecutor)
	if !ok {
		return nil, fmt.Errorf(
			"this connection does not support native queries",
		)
	}
	return nq, nil
}
