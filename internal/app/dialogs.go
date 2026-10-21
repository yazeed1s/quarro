package app

// SaveFileDialog opens a native "Save File" dialog and returns the chosen
// path, or an empty path with a nil error if the user cancelled. It is
// injected rather than called directly so internal/app stays buildable and
// testable without Wails' GUI backend, which needs cgo/GTK on Linux even
// just to import. See main.go for the real implementation and
// internal/app/export_service_test.go for a fake used in tests.
type SaveFileDialog interface {
	SaveFile(
		defaultName, filterLabel, filterPattern string,
	) (string, error)
}

// OpenFileDialog is the same idea for choosing an existing file to open.
type OpenFileDialog interface {
	OpenFile(title, filterLabel, filterPattern string) (string, error)
}
