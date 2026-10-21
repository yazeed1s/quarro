package provider

// Blank imports so this package's own tests, built as an isolated binary,
// see every built-in engine registered with internal/database/driver the
// same way the real application does via main.go's blank import block.
// provider.go itself imports none of these; only its tests need them.
import (
	_ "github.com/yazeed1s/quarro/internal/database/mongodb"
	_ "github.com/yazeed1s/quarro/internal/database/mysql"
	_ "github.com/yazeed1s/quarro/internal/database/postgres"
	_ "github.com/yazeed1s/quarro/internal/database/redis"
	_ "github.com/yazeed1s/quarro/internal/database/sqlite"
)
