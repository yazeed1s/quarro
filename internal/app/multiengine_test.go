package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/jobs"
)

func TestConcurrentEngineSessions(t *testing.T) {
	if os.Getenv("QUARRO_TEST_POSTGRES_URL") == "" ||
		os.Getenv("QUARRO_TEST_MYSQL_URL") == "" {
		t.Skip("requires isolated PostgreSQL and MySQL URLs")
	}
	svc := newTestService(t)
	defer svc.sessions.CloseAll()
	queries := NewQueriesService(svc.sessions, jobs.NewRegistry())
	profiles := []connections.Input{
		{
			Profile: connections.Profile{
				ID:     "sqlite",
				Name:   "SQLite",
				Engine: connections.EngineSQLite,
				Path:   filepath.Join(t.TempDir(), "local.db"),
			},
		},
	}
	for _, engine := range []string{"postgres", "mysql"} {
		p, e := provider.ImportURL(
			os.Getenv(
				"QUARRO_TEST_" + strings.ToUpper(engine) + "_URL",
			),
		)
		if e != nil {
			t.Fatal(e)
		}
		p.ID = engine
		p.Name = engine
		profiles = append(profiles, p)
	}
	for _, p := range profiles {
		if _, e := svc.SaveProfile(p); e != nil {
			t.Fatal(e)
		}
		result, e := svc.Connect(p.ID, p.ID)
		if e != nil || !result.OK {
			t.Fatalf("connect %s: %+v %v", p.ID, result, e)
		}
	}
	var wg sync.WaitGroup
	for _, p := range profiles {
		wg.Add(1)
		go func(p connections.Input) {
			defer wg.Done()
			result, e := queries.ExecuteDocument(
				p.ID,
				"same-document-id",
				"job-"+p.ID,
				"CREATE TEMPORARY TABLE private_session(id INTEGER); INSERT INTO private_session VALUES(42); SELECT * FROM private_session",
				10,
				10,
			)
			if e != nil || len(result.Results) != 3 ||
				result.Results[2].Kind != "rows" {
				t.Errorf("%s isolation: %+v %v", p.ID, result, e)
			}
		}(p)
	}
	wg.Wait()
	for _, engine := range []string{"postgres", "mysql"} {
		target := engine
		result, e := svc.OpenDatabase(engine, "other-"+engine, target)
		if e != nil || !result.OK {
			t.Fatalf("switch: %+v %v", result, e)
		}
		source, _ := svc.sessions.Profile(engine)
		other, _ := svc.sessions.Profile("other-" + engine)
		if source.Database == other.Database ||
			other.Database != target {
			t.Fatal("database switch changed source")
		}
		statement := "SELECT current_database() AS name"
		if engine == "mysql" {
			statement = "SELECT DATABASE() AS name"
		}
		r, e := queries.ExecuteDocument(
			"other-"+engine,
			"same-document-id",
			"target-"+engine,
			statement,
			10,
			10,
		)
		if e != nil || r.Results[0].Rows[0]["name"] != target {
			t.Fatalf("wrong target: %+v %v", r, e)
		}
		r, e = queries.ExecuteDocument(
			engine,
			"same-document-id",
			"original-"+engine,
			"SELECT * FROM private_session",
			10,
			10,
		)
		if e != nil || r.Results[0].Kind != "rows" {
			t.Fatalf("source connection changed: %+v %v", r, e)
		}
	}
	svc.Disconnect("postgres")
	for _, engine := range []string{"sqlite", "mysql"} {
		r, e := queries.ExecuteDocument(
			engine,
			"same-document-id",
			"remaining-"+engine,
			"SELECT * FROM private_session",
			10,
			10,
		)
		if e != nil || r.Results[0].Kind != "rows" {
			t.Fatalf("disconnect crossed sessions: %+v %v", r, e)
		}
	}
}

func TestReadOnlyServerSessionsRefuseDirectSQLWrites(t *testing.T) {
	for _, engine := range []string{"postgres", "mysql"} {
		t.Run(engine, func(t *testing.T) {
			raw := os.Getenv("QUARRO_TEST_" + strings.ToUpper(engine) + "_URL")
			if raw == "" {
				t.Skip("requires an isolated " + engine + " URL")
			}
			input, err := provider.ImportURL(raw)
			if err != nil {
				t.Fatal(err)
			}
			input.ID = engine + "-readonly-contract"
			input.Name = engine + " readonly contract"
			input.ReadOnly = true

			svc := newTestService(t)
			defer svc.sessions.CloseAll()
			if _, err := svc.SaveProfile(input); err != nil {
				t.Fatal(err)
			}
			connected, err := svc.Connect(input.ID, input.ID)
			if err != nil || !connected.OK {
				t.Fatalf("connect: %+v %v", connected, err)
			}
			queries := NewQueriesService(svc.sessions, jobs.NewRegistry())
			read, err := queries.ExecuteDocument(input.ID, "doc", "read", "SELECT 1 AS value", 10, 10)
			if err != nil || len(read.Results) != 1 || read.Results[0].Kind != "rows" {
				t.Fatalf("read: %+v %v", read, err)
			}
			write, err := queries.ExecuteDocument(input.ID, "doc", "write", "CREATE TABLE quarro_readonly_contract (id INT)", 10, 10)
			if err != nil || len(write.Results) != 1 || write.Results[0].Kind != "error" {
				t.Fatalf("write was not refused: %+v %v", write, err)
			}
			override, err := queries.ExecuteDocument(input.ID, "doc", "override", "SET SESSION TRANSACTION READ WRITE", 10, 10)
			if err != nil || len(override.Results) != 1 || override.Results[0].Kind != "error" || !strings.Contains(override.Results[0].Error, "changing its read-only mode") {
				t.Fatalf("override was not refused by backend guard: %+v %v", override, err)
			}
		})
	}
}
