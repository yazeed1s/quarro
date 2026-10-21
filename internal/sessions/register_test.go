package sessions

// Blank import so this package's tests, built as an isolated binary, see
// SQLite registered with internal/database/driver: Registry.Install calls
// provider.Get(connections.EngineSQLite) internally, and this package's
// production code otherwise never imports a concrete engine package.
import _ "github.com/yazeed1s/quarro/internal/database/sqlite"
