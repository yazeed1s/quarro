package app

import (
	"context"
	"strings"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/model"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/sessions"
)

// stubNative is a native-query engine whose every query "writes", counting
// how often ExecuteNative actually ran.
type stubNative struct {
	stubKeyEditor
	runs int
}

func (s *stubNative) ExecuteNative(context.Context, any, string, int) model.ResultSet {
	s.runs++
	return model.ResultSet{Kind: "rows"}
}

func (*stubNative) NativeQueryWrites(context.Context, any, string) (bool, error) {
	return true, nil
}

func TestExecuteNativeRefusesReadOnlyWrites(t *testing.T) {
	for _, ro := range []bool{true, false} {
		reg := sessions.NewRegistry()
		eng := &stubNative{}
		reg.InstallProvider(
			"s1",
			connections.Profile{ID: "p1", Engine: connections.EngineRedis, ReadOnly: ro},
			reg.BeginConnect("s1"),
			nil,
			eng,
		)
		svc := NewQueriesService(reg, jobs.NewRegistry())
		res, err := svc.ExecuteNativeDocument("s1", "doc", "job", `{"command":"SET k v"}`, 10, 5)
		if err != nil || len(res.Results) != 1 {
			t.Fatalf("ro=%v: %+v, %v", ro, res, err)
		}
		got := res.Results[0]
		if ro && (got.Kind != "error" || !strings.Contains(got.Error, "read-only") || eng.runs != 0) {
			t.Fatalf("read-only: %+v, runs=%d, want a refusal before execution", got, eng.runs)
		}
		if !ro && (got.Kind != "rows" || eng.runs != 1) {
			t.Fatalf("writable: %+v, runs=%d", got, eng.runs)
		}
	}
}
