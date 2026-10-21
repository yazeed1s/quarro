# Writing a database provider

This is what I'd tell myself before adding a sixth engine, after doing it five times (SQLite, PostgreSQL, MySQL, MongoDB, Redis). The short version is that an engine is one Go package that registers itself from `init()`, implements only the capabilities the engine honestly has, and gets tested against a real server. The app finds out what it can do by asking, not by checking engine names.

## the pieces

`internal/database/capability` holds the small interfaces and imports no engine, `internal/database/driver` is a registry engines put themselves into, and `internal/database/provider` re-exports the interfaces under older names, routes a profile to its engine through the registry, and does validation and URL import. Each engine lives in `internal/database/<engine>/` and `main.go` blank-imports it, which is the one place that lists every engine by name:

```go
_ "github.com/yazeed1s/quarro/internal/database/redis"
```

Forget that import and the engine compiles fine and just never shows up in the app, and your own package tests still pass, which is confusing the first time. Add the same blank import to `internal/database/provider/register_test.go` and `internal/app/register_test.go` too, because those test binaries are built alone and only see engines they import.

## pick capabilities honestly

The interfaces are in `capability/*.go`, one file each, and they are small on purpose: `Connector`, `Discoverer`, `LazyDiscoverer`, `RoutineDiscoverer`, `ConstraintDiscoverer`, `DefinitionDiscoverer`, `SQLExecutor`, `Browser`, `Editor`, `DocumentEditor`, `Transactor`, `KeyValueStore`, `KeyValueEditor`, `Aggregator`, `QueryCompiler`, `Explainer`, `IdentifierFormatter`, `NativeQueryExecutor`, `NativeWriteDetector`, `ReadOnlyEnforcer`. `SQLProvider` is just the composition every SQL engine satisfies.

Implement the ones your engine actually supports and skip the rest. If implementing one means inventing a concept the engine doesn't have (a WHERE clause for Redis, a JOIN for a document store) then don't. Right now it looks like this:

| | sqlite | postgres / mysql | mongodb | redis |
| --- | --- | --- | --- | --- |
| Connector, Discoverer | yes | yes | yes | yes |
| SQLProvider (executor, browser, editor, transactor, compiler, aggregator, identifiers) | yes | yes | no | no |
| Lazy / Routine / Constraint / Definition discoverers | definition only | yes | no | no |
| Explainer | yes (no analyze) | yes | yes | no |
| Browser, Aggregator | yes | yes | yes (no joins, no group by) | no |
| DocumentEditor | no | no | yes | no |
| KeyValueStore, KeyValueEditor | no | no | no | yes |
| NativeQueryExecutor | no | no | pipelines | redis commands (+ NativeWriteDetector) |

The table drifts, so trust the `var _ capability.X = Provider{}` block at the top of each engine's `driver.go` over this. Put yours in the same place, it's both documentation and a build failure the day someone breaks a method signature.

MongoDB implements `Browser` because paging a collection with a filter, sort and skip/limit is a real Mongo operation. Redis doesn't, because a keyspace has no offset and no sort (SCAN is a cursor), so `KeyValueStore` got added instead. That's the rule I care about most here: **when your engine doesn't fit an existing capability, add a new small interface, don't bend an old one and don't put `if engine == "x"` in `internal/app`.** Follow the existing shape, a few methods, `Connection` or `Handle` as the resource, `model.ResultSet` or a plain error out, then add a field for it to `capability.Capabilities` and a line in `Of`.

## Handle and Connection are `any`

`capability.Handle` and `capability.Connection` are both `= any`, and `internal/sessions` stores and passes them around without knowing what's inside. Only your package creates one or asserts it back, which is why Mongo and Redis came in without sessions changing at all.

`Open(connections.Input)` connects and returns whatever "the thing I'm connected to" is (a `*sql.DB`, a `*mongo.Client`, a `*redis.Client`). `OpenConnection(ctx, Handle)` checks out one unit of work, for `database/sql` that is a real `db.Conn(ctx)` and the document keeps it pinned (transactions live there), but for a client that is already its own pool it can just return the handle again, see `mongodb.go` or `redis.go` for the one-liner. `DiscardConnection` poisons a connection whose outcome is unknown (a cancelled statement) so it never goes back to the pool, no-op if there's nothing per connection. `Close` closes the handle. Every other method asserts on its first line:

```go
func (Provider) Scan(ctx context.Context, h capability.Connection, prefix string, group bool, limit int) model.ResultSet {
	c := h.(*redis.Client)
	// ...
}
```

## register a driver

Each engine has a `driver.go` with a small `Driver` type and an `init()` that calls `driver.Register(Driver{})`. The driver has three methods. `Metadata()` returns a name (must match the `connections.Engine` value), display name, icon key, the CodeMirror SQL dialect if there is one, and a `ConfigSchema` describing the connection form fields (`host`, `port`, `password`, ...), and the frontend renders the connection form from that, so you don't write a form. `New()` returns your provider. `Capabilities()` is usually `capability.Of(New())` plus the quirk flags that are not interfaces, like this from redis:

```go
func (Driver) Capabilities() capability.Capabilities {
	c := capability.Of(New())
	c.NativeQueryLanguage = "redis-command"
	return c
}
```

The quirk flags are `SupportsAnalyze`, `RequiresDelimiterWrapping`, `SupportsJoins`, `SupportsGroupBy` and `NativeQueryLanguage`, read the comments in `capability/capabilities.go`. The frontend gets all of this per session through `QueriesService.SessionCapabilities` and hides buttons based on it (`session.capabilities.explainer` and so on), so if you set the flags right most of the UI just works.

## reuse dialect, if you're SQL

`internal/database/dialect` has everything shared by the `database/sql` engines: the edit statement builder, the visual query SELECT compiler, the `:name` parameter scanner and binder, the statement splitter, row scanning and error helpers, TLS config, SSH helpers and qualified object ids. A fourth SQL dialect should reuse most of it, look at how thin postgres and mysql are. A non-SQL engine will still probably want `dialect.TLSConfig` (Postgres, MySQL, Mongo and Redis all use it the same way).

Before writing your own version of something in dialect, check whether the difference is really about your SQL dialect (then it belongs in your package) or it's generic and should go into dialect with a parameter. I'd lean to the second more often than feels natural.

## profile and validation

Add an `EngineXxx` constant to `internal/connections/store.go`. Reuse the existing `Profile` fields (host, port, username, password, database, TLS, socket) when they fit, Redis puts its numbered db index in `Database` as a string and nobody died. A field that really has no home goes in `Profile.Config` (a string map) and you declare it in your `ConfigSchema`, `internal/app/config_schema.go` then checks required ones for you.

`dialect.ValidateProfile` is built for SQL and always requires a username. If your rules are different write `ValidateProfile(connections.Profile) error` in your package and add a case for it in `provider.Validate` (`internal/database/provider/open.go`), the same as mongodb and redis. That switch and `ImportURL` (which only takes postgres:// and mysql:// for now) are the only places outside your package that still know engine names (validation could probably live on the driver too, hasn't been worth it yet).

## frontend

Most of it comes for free from the driver metadata and capabilities, but a few things are still hardcoded. Add your engine to the `Engine` union in `frontend/src/lib/state/types.ts`, look at `engineChanged` in `ConnectionManager.svelte` for anything the schema doesn't cover (like whether the socket or TLS section shows), and add an icon for your `Icon` key.

If your objects aren't SQL tables, give `model.Table` a `Kind` (mongo uses `"collection"`, redis `"keyspace"`) and reuse the object browser tree, it's already generic over "a list of named objects". If your results aren't rows, use the tagged union in `model.ResultSet` (it already has `Documents` and `Entries` for exactly this) and add a branch to the renderer instead of faking rows. The `keyvalue` branch is the minimal example, a plain table, not a new component.

## tests

Same rule as the rest of the app, a real server in a throwaway container and never a mock. [testing.md](testing.md) explains how the containers work. For a new engine:

1. Write `TestContract` in `internal/database/<engine>/<engine>_test.go` that connects, seeds a bit of data, exercises every capability you claimed, and calls `t.Skip` when `QUARRO_TEST_<ENGINE>_URL` is empty.
2. Add a mode to `scripts/test-engine.sh`, a `start_<engine>` function shaped like `start_mongo`/`start_redis` (random container name, `127.0.0.1::<port>`, `wait_for` on a real query, append to `containers` so the trap removes it) and a case that exports the env var and runs `go test`. Add it to `all` too.
3. If you added UI, a Playwright spec in `frontend/tests/` that skips without the env var, plus a seeded script mode for it.
4. `go build ./...`, `go vet ./...`, `gofmt -l .` and `go test -race ./internal/...` all clean, and `npm run check` in frontend.

Adding the engine to `dev/docker-compose.yml` with some seed data is optional, nice for poking at by hand but it proves nothing, the contract test is what proves it.
