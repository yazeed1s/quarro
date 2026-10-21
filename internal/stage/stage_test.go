package stage

import (
	"testing"
	"testing/fstest"
)

func TestReadAndNamespaces(t *testing.T) {
	for _, mode := range []string{"prod", "preview", "test"} {
		got, err := Read(
			fstest.MapFS{
				"frontend/dist/app-stage.json": {
					Data: []byte(`"` + mode + `"`),
				},
			},
		)
		if err != nil || got != mode {
			t.Fatalf("%s: %s %v", mode, got, err)
		}
	}
	if Suffix("preview") == Suffix("prod") ||
		Suffix("test") == Suffix("prod") {
		t.Fatal("stage stores overlap")
	}
	if _, err := Read(fstest.MapFS{}); err == nil {
		t.Fatal("missing build configuration silently accepted")
	}
	if _, err := Read(fstest.MapFS{"frontend/dist/app-stage.json": {Data: []byte(`"prodd"`)}}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
