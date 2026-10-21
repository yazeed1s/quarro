# Contributing

PRs and issues go to [github.com/yazeed1s/quarro](https://github.com/yazeed1s/quarro). For anything bigger than a bug fix, open an issue first and say what you want to change, mostly so we don't both spend a weekend on it and so I can tell you if there's a reason it's the way it is (there usually is, sometimes it's a bad one).

Setup is in the [README](../README.md#build-from-source), `wails3 task dev` for the app with hot reload and `docker compose -f dev/docker-compose.yml up -d --wait` for something to connect to.

## before you open a PR

```sh
go build ./... && go vet ./... && gofmt -l .
go test -race ./internal/...
scripts/test-engine.sh all            # if you touched anything under internal/database or internal/app
cd frontend && npm run check && npm run test:unit && npm run format:check
```

`gofmt -l .` should print nothing. If you changed something an engine does, run that engine's mode of `scripts/test-engine.sh` at least, and if you changed UI, the Playwright suite (`QUARRO_BROWSER_TESTS=1 scripts/test-engine.sh all`, or `npm test` in frontend for the sqlite-only part). [testing.md](testing.md) has the details. Changes to execution, cancellation, transactions or anything that writes need tests for the failure path too, not only a happy path SQL string compare, that's where the real bugs were every time.

Don't upgrade dependencies as a side effect. Wails is pinned to v3.0.0-beta.22 for Go, the CLI and the frontend runtime, and they have to move together, a separate PR for that is fine.

## where things go

`internal/app` is the services the frontend calls (thin, orchestration only), `internal/sessions` owns pools and the pinned connection per document, `internal/jobs` owns query cancellation, `internal/database` is capabilities, the driver registry and one package per engine, `internal/connections` and `internal/secrets` keep profiles and passwords apart (passwords never go into a profile file, browser storage or logs). On the frontend, `components/` is shared view stuff, `features/` is one folder per feature with its own local state, `state/workspace.svelte.ts` is the durable workspace state and `state/backend.ts` is the only thing that talks to the generated Wails bindings.

Database SQL stays in the engine layer, a component never builds SQL and never imports bindings directly, it goes through workspace actions. If something only some engines can do, check the session's capabilities, never the engine name, see [providers.md](providers.md). Adding a new engine is its own whole thing and that doc covers it.

## Go style

`gofmt`, `ctx context.Context` first, context-aware database calls everywhere, and channel waits that can block on shutdown also select on `ctx.Done()`. Return errors with short context and `%w`, don't panic on anything a user or a network can cause. Close rows, cancel contexts, release or discard connections on every exit, and check `rows.Err()`. If a write or a commit loses its connection the outcome is unknown, keep it unknown instead of guessing success or failure.

Names are short for locals (`ctx`, `err`, `db`, `tx`, `req`, `res`, `buf`, `p`, `r`, `n`), recognizable within the few lines they live in, and descriptive for exported things (`ApplyTableEdits`, `BrowseTable`, `RollbackTransaction`). If no short name carries the meaning use a longer one with a short comment. Don't rename shipped identifiers just to match this, and the Wails-bound parameters (`sessionID`, `documentID`, `jobID`) stay as they are. Files are named after their feature (`schema.go`, `query.go`), please no new `utils.go`.

Comments are short, simple English, and say why, who owns what, ordering, or what breaks, not what the syntax does:

```go
// Keep the connection pinned so this document retains its transaction.
```

## frontend style

Svelte 5 runes, typed `$props()`, `$state`, `$derived`, derive things instead of syncing duplicate state through effects, and use `$effect` only for real synchronization with cleanup. Transient form and menu state stays in the component, durable document state goes in the workspace. Documents stay mounted across tab switches to keep editor state, so check visibility before focusing anything, and guard async results with the job id or session generation so a late response doesn't overwrite newer state.

Formatting is oxfmt (single quotes, trailing commas, 180 columns), run `npm run format` from `frontend/`. Colors come from the semantic theme tokens in `frontend/src/lib/styles/theme.css`, and the result grid's virtualization depends on row height from `state/appearance.ts`, so use that and don't hardcode one. The longer UI guide is in `ref/skills/sqlview-ui-ux/SKILL.md` (old name, same app).

## PRs and commits

Keep a PR to one thing. Commit messages in plain English, imperative is fine, say why if it's not obvious from the diff. Screenshots for UI changes help a lot, both themes if you changed colors. User-visible limits or unfinished pieces go in the roadmap (`ref/todo.md`), not in a second checklist somewhere.

## reporting bugs

Say which OS and version, which engine and server version, and what you did. For "it won't open" problems on macOS or Windows read [install.md](install.md) first, unsigned app warnings are expected, but "is damaged and can't be opened" on macOS is not and I want to hear about it.
