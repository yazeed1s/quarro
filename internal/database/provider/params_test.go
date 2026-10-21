package provider

import (
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
)

func TestPlaceholdersAndBindParams(t *testing.T) {
	cases := []struct {
		engine connections.Engine
		sql    string
		values map[string]any
		names  []string
		bound  string
		args   []any
	}{
		{
			connections.EngineSQLite, "SELECT * FROM users WHERE id = :id",
			map[string]any{"id": "7"}, []string{"id"},
			"SELECT * FROM users WHERE id = ?", []any{int64(7)},
		},
		{
			connections.EnginePostgres, "SELECT * FROM t WHERE a = :x AND b = :y",
			map[string]any{"x": "1", "y": "hi"}, []string{"x", "y"},
			"SELECT * FROM t WHERE a = $1 AND b = $2", []any{int64(1), "hi"},
		},
		{
			connections.EngineMySQL, "SELECT :n + :n",
			map[string]any{"n": "3"}, []string{"n"},
			"SELECT ? + ?", []any{int64(3), int64(3)},
		},
		{
			connections.EngineSQLite, "SELECT :price",
			map[string]any{"price": "9.5"}, []string{"price"},
			"SELECT ?", []any{9.5},
		},
	}
	for _, c := range cases {
		p := mustGet(t, c.engine)
		names, err := p.Placeholders(c.sql)
		if err != nil {
			t.Fatalf("%s: %v", c.engine, err)
		}
		if len(names) != len(c.names) {
			t.Fatalf("%s: names=%v want %v", c.engine, names, c.names)
		}
		for i, n := range names {
			if n != c.names[i] {
				t.Fatalf(
					"%s: names=%v want %v",
					c.engine,
					names,
					c.names,
				)
			}
		}
		bound, args, err := p.BindParams(c.sql, c.values)
		if err != nil {
			t.Fatalf("%s: %v", c.engine, err)
		}
		if bound != c.bound {
			t.Fatalf("%s: bound=%q want %q", c.engine, bound, c.bound)
		}
		if len(args) != len(c.args) {
			t.Fatalf("%s: args=%v want %v", c.engine, args, c.args)
		}
		for i := range args {
			if args[i] != c.args[i] {
				t.Fatalf(
					"%s: args[%d]=%v (%T) want %v (%T)",
					c.engine,
					i,
					args[i],
					args[i],
					c.args[i],
					c.args[i],
				)
			}
		}
	}
}

func TestBindParamsMissingValue(t *testing.T) {
	p := mustGet(t, connections.EngineSQLite)
	if _, _, err := p.BindParams("SELECT :missing", map[string]any{}); err == nil {
		t.Fatal("expected an error for a missing parameter value")
	}
}

func TestBindParamsIgnoresLiteralsAndCasts(t *testing.T) {
	p := mustGet(t, connections.EnginePostgres)
	names, err := p.Placeholders(
		"SELECT 'not :fake', a::text FROM t WHERE b = :real",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "real" {
		t.Fatalf("names=%v", names)
	}
	bound, args, err := p.BindParams(
		"SELECT 'not :fake', a::text FROM t WHERE b = :real",
		map[string]any{"real": "5"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bound != "SELECT 'not :fake', a::text FROM t WHERE b = $1" {
		t.Fatalf("bound=%q", bound)
	}
	if len(args) != 1 || args[0] != int64(5) {
		t.Fatalf("args=%v", args)
	}
}

func TestBindParamsNoPlaceholders(t *testing.T) {
	p := mustGet(t, connections.EngineMySQL)
	bound, args, err := p.BindParams("SELECT 1", nil)
	if err != nil || bound != "SELECT 1" || args != nil {
		t.Fatalf("bound=%q args=%v err=%v", bound, args, err)
	}
}

func TestCoerceParamNull(t *testing.T) {
	p := mustGet(t, connections.EngineSQLite)
	bound, args, err := p.BindParams(
		"SELECT :v",
		map[string]any{"v": nil},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bound != "SELECT ?" || len(args) != 1 || args[0] != nil {
		t.Fatalf("bound=%q args=%v", bound, args)
	}
}
