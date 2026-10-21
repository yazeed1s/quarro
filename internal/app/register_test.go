package app

// Blank imports so this package's tests, built as an isolated binary, see
// every non-SQLite built-in engine registered with internal/database/driver.
// SQLite is already registered via queries_service.go's own import of
// internal/database/sqlite for SplitStatements/MaxResultBytes; the others
// are needed here because internal/app's production code otherwise never
// imports a concrete engine package (see internal/database/provider.Get).
import (
	_ "github.com/yazeed1s/quarro/internal/database/mongodb"
	_ "github.com/yazeed1s/quarro/internal/database/mysql"
	_ "github.com/yazeed1s/quarro/internal/database/postgres"
	_ "github.com/yazeed1s/quarro/internal/database/redis"
)
