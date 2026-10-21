package main

import (
	"embed"
	"log"
	"net/http"
	// Registers /debug/pprof handlers on http.DefaultServeMux.
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"

	"github.com/wailsapp/wails/v3/pkg/application"

	quarroapp "github.com/yazeed1s/quarro/internal/app"
	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/demo"
	"github.com/yazeed1s/quarro/internal/desktop"
	"github.com/yazeed1s/quarro/internal/jobs"
	"github.com/yazeed1s/quarro/internal/secrets"
	"github.com/yazeed1s/quarro/internal/sessions"
	"github.com/yazeed1s/quarro/internal/stage"
	"github.com/yazeed1s/quarro/internal/windowprefs"

	// Each engine package registers itself with internal/database/driver
	// from its own init(). This is the one place that lists every built-in
	// engine by name; internal/database/provider.Get and .Validate look
	// engines up in that registry instead of importing them directly.
	_ "github.com/yazeed1s/quarro/internal/database/mongodb"
	_ "github.com/yazeed1s/quarro/internal/database/mysql"
	_ "github.com/yazeed1s/quarro/internal/database/postgres"
	_ "github.com/yazeed1s/quarro/internal/database/redis"
	_ "github.com/yazeed1s/quarro/internal/database/sqlite"
)

// dist is the built frontend (frontend/dist).
//
//go:embed all:frontend/dist
var dist embed.FS

// cfg is Wails' project config; its info.version is the app's
// version (see scripts/bump-version.sh).
//
//go:embed build/config.yml
var cfg []byte

func main() {
	// Trade CPU for a smaller steady-state heap: collect twice as often as
	// the Go default (GOGC=100) and never let the heap exceed 256 MiB.
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(256 << 20)

	mode, err := stage.Read(dist)
	if err != nil {
		log.Fatal(err)
	}
	ver, err := quarroapp.ParseBuildVersion(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if mode != "prod" {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}
	sfx := stage.Suffix(mode)
	ps, err := connections.NewStoreForStage(
		stage.AppDir,
		sfx,
	)
	if err != nil {
		log.Fatal(err)
	}
	if mode != "prod" {
		if err := seedDemoDatabase(ps, sfx); err != nil {
			log.Printf("demo database setup failed: %v", err)
		}
	}

	sr := sessions.NewRegistry()
	jr := jobs.NewRegistry()
	csvc := quarroapp.NewConnectionsService(
		ps,
		secrets.NewStoreForNamespace(stage.AppDir+sfx),
		sr,
	)
	qsvc := quarroapp.NewQueriesService(
		sr,
		jr,
	)
	esvc := quarroapp.NewExportService(
		sr,
		jr,
		desktop.Dialogs{},
	)
	isvc := quarroapp.NewImportService(
		sr,
		jr,
		desktop.Dialogs{},
	)
	// wst persists window.json; wp is what this launch builds the window from.
	wst, err := windowprefs.NewStoreForStage(stage.AppDir, sfx)
	if err != nil {
		log.Fatal(err)
	}
	wp, err := wst.Load()
	if err != nil {
		log.Printf("window preferences unreadable, using defaults: %v", err)
	}
	dsvc := quarroapp.NewDesktopService(
		wst,
		wp,
		runtime.GOOS,
		desktop.Window{},
		desktop.Clipboard{},
	)
	asvc := quarroapp.NewAppService(
		quarroapp.AppInfo{
			Version: ver,
			Stage:   mode,
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		quarroapp.LatestReleaseAPI,
	)

	app := application.New(application.Options{
		Name:        "Quarro",
		Description: "A desktop database workspace",
		Icon:        desktop.Icon(wp.AppIcon),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(dist),
		},
		Services: []application.Service{
			application.NewService(csvc),
			application.NewService(qsvc),
			application.NewService(esvc),
			application.NewService(isvc),
			application.NewService(dsvc),
			application.NewService(asvc),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	desktop.NewMainWindow(app, wp)
	desktop.InstallMacMenu(app)

	app.OnShutdown(func() {
		jr.CloseAll()
		sr.CloseAll()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// seedDemoDatabase provisions a sample SQLite database on first launch when no saved profiles exist, giving a fresh install queryable data without setup or credentials.
func seedDemoDatabase(ps *connections.Store, sfx string) error {
	pl, err := ps.List()
	if err != nil {
		return err
	}
	if len(pl) > 0 {
		return nil
	}
	p, err := demo.EnsureDatabaseForStage(stage.AppDir, sfx)
	if err != nil {
		return err
	}
	_, err = ps.Save(connections.Profile{
		ID:          "demo-sqlite",
		Name:        "Sample database",
		Engine:      connections.EngineSQLite,
		Database:    "demo.db",
		Schema:      "main",
		Environment: "Local",
		Folder:      "Sample",
		Favorite:    true,
		Path:        p,
	})
	return err
}
