package capability

import "github.com/yazeed1s/quarro/internal/database/model"

// QueryCompiler compiles the Visual Query Builder's AST into this engine's
// own SQL dialect. Only an engine with a real SQL compiler implements this.
type QueryCompiler interface {
	CompileSelect(model.VisualQuery) (string, error)
}
