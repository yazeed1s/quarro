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

// stubKeyEditor counts ApplyKeyEdit calls, so a test can prove a refused
// edit never reached the engine.
type stubKeyEditor struct{ calls int }

func (*stubKeyEditor) Open(connections.Input) (any, error) { return nil, nil }
func (*stubKeyEditor) OpenConnection(context.Context, any) (any, error) {
	return struct{}{}, nil
}
func (*stubKeyEditor) DiscardConnection(any) error { return nil }
func (*stubKeyEditor) Close(any) error             { return nil }
func (s *stubKeyEditor) ApplyKeyEdit(
	context.Context,
	any,
	model.KeyEdit,
) (model.KeyEditResult, error) {
	s.calls++
	return model.KeyEditResult{Applied: true}, nil
}

func TestApplyKeyEditRefusesReadOnly(t *testing.T) {
	for _, ro := range []bool{true, false} {
		reg := sessions.NewRegistry()
		eng := &stubKeyEditor{}
		reg.InstallProvider(
			"s1",
			connections.Profile{ID: "p1", Engine: connections.EngineRedis, ReadOnly: ro},
			reg.BeginConnect("s1"),
			nil,
			eng,
		)
		svc := NewQueriesService(reg, jobs.NewRegistry())
		res, err := svc.ApplyKeyEdit("s1", "doc", model.KeyEdit{Key: "k", Op: "del"})
		if ro {
			if err == nil || !strings.Contains(err.Error(), "read-only") || eng.calls != 0 {
				t.Fatalf("read-only: res=%+v err=%v calls=%d, want refusal before the engine", res, err, eng.calls)
			}
			continue
		}
		if err != nil || !res.Applied || eng.calls != 1 {
			t.Fatalf("writable: res=%+v err=%v calls=%d", res, err, eng.calls)
		}
	}
}
