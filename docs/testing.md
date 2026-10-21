# Testing against real databases

Every engine in Quarro is tested against a real server, never a mock, because most bugs live in the gap between what a driver says and what the server actually does (MySQL answering ping before root's grants exist is a good example) and a mock would just agree with me. So the rule is simple, if a test talks to a database it talks to a real one in a throwaway container, and if no container is there it skips.

There are two separate Docker setups and they are easy to mix up. `scripts/test-engine.sh` makes disposable servers for tests and deletes them after, and `dev/docker-compose.yml` is a long running seeded stack for clicking around in the app by hand. Tests never use the compose stack.

## how the contract tests find a server

MongoDB and Redis have a `TestContract` (plus narrower tests) in their own package, `internal/database/<engine>/<engine>_test.go`, and the SQL engines share `TestRemoteContracts` in `internal/database/provider/provider_test.go`, which loops over postgres and mysql and builds the env var name from the engine. All of them read the env var first and call `t.Skip` when it's empty, so a plain `go test ./internal/...` on a laptop without Docker just passes with skips. The vars are:

| Var | Read by |
| --- | --- |
| `QUARRO_TEST_POSTGRES_URL`, `QUARRO_TEST_POSTGRES_SOCKET` | `TestRemoteContracts` in provider, `internal/app/multiengine_test.go` |
| `QUARRO_TEST_MYSQL_URL`, `QUARRO_TEST_MYSQL_SOCKET` | same two |
| `QUARRO_TEST_MARIADB_URL` | `TestMariaDBExplainContract` in provider |
| `QUARRO_TEST_MONGO_URL`, `QUARRO_TEST_MONGO_DB` | mongodb package, and the mongo Playwright specs |
| `QUARRO_TEST_REDIS_URL` | redis package, and the redis Playwright specs |
| `QUARRO_TEST_SSH_*` | `TestSSHTunnelContract` in postgres and mysql |
| `QUARRO_BROWSER_TESTS=1` | makes the script also run Playwright |

You can set these yourself and point them at any server you like, but the script does it for you and that's what I'd use.

## scripts/test-engine.sh

```sh
scripts/test-engine.sh sql      # postgres 17 + mysql 8.4 with TLS, then go test ./internal/...
scripts/test-engine.sh mongo    # mongo 7, mongodb package only
scripts/test-engine.sh redis    # redis 7, redis package only
scripts/test-engine.sh mariadb  # mariadb 11, the ANALYZE FORMAT=JSON explain path
scripts/test-engine.sh ssh      # postgres + mysql behind an ssh jump host
scripts/test-engine.sh all      # everything at once, one go test pass over all five engines
QUARRO_BROWSER_TESTS=1 scripts/test-engine.sh all   # same, then the full Playwright suite
```

There are also single-scenario modes for the browser tests (`mongo-explain`, `mongo-chart`, `mongo-json-tree`, `mongo-document-edit`, `mongo-native-query`, `redis-inspect`, `redis-key-edit`, `redis-native-command`), each one seeds just the data its spec needs and runs `npm test -- -g "<that spec>"`. Running the script with no argument prints the full list.

Every container gets a name like `quarro-redis-<pid>-<random>` and publishes its port as `127.0.0.1::6379`, so Docker picks a free host port and the script reads it back with `docker port`. That means you can run two modes at the same time, or have the dev compose stack up, and nothing collides. The names go into a `containers` array (networks into `networks`) and a `trap cleanup EXIT` removes all of them when the script exits, also on Ctrl-C or a failed test, so there is nothing to clean up after. The cleanup function ends with `return 0` on purpose, because a trap's exit status replaces the script's, and an empty work dir would otherwise turn a clean usage error into exit 1. past me learned that one the annoying way.

Waiting for readiness goes through `wait_for`, which polls a check command once a second for up to 120 seconds. The check is a real authenticated query where it matters (`SELECT 1` as root for MySQL and MariaDB) and not just a ping, because both images answer ping during their own init-then-restart cycle before the server will actually accept you. Postgres uses `pg_isready`, Mongo `db.runCommand({ping:1})` through mongosh, Redis `redis-cli ping`, those seem fine.

### sql mode and TLS

This one does the most work. It makes a temp dir, generates a throwaway CA and a server cert for `localhost`/`127.0.0.1` with openssl (valid one day), starts postgres:17 and mysql:8.4 with their socket directories bind-mounted from the temp dir, copies the certs into each container and chowns them to the server user (copying instead of mounting so ownership works on any host), turns TLS on (`ALTER SYSTEM SET ssl = on` for Postgres, a `conf.d` file for MySQL), restarts both and waits again. The URLs it exports use `sslmode=verify-full` with the private CA, so the TLS tests actually check the certificate chain and hostname instead of just encrypting. The mounted sockets are what `QUARRO_TEST_*_SOCKET` point at, for the unix socket connection path.

### ssh mode

Postgres and MySQL go on a private Docker network with no published port at all, so the host can't reach them directly, and next to them an `alpine:3` container becomes the jump host (openssh installed at runtime, a `tunnel` user with a password, plus an ed25519 key generated on the host and authorized inside). Only the jump host publishes port 22. A query that succeeds therefore proves the tunnel works and not some accidental direct route, which was the whole point. Alpine ships `AllowTcpForwarding no` uncommented and OpenSSH takes the first occurrence of a directive, so appending `yes` does nothing and every tunnel fails with "administratively prohibited" while login works fine. The script edits the line in place with sed. that took a while to find ngl.

### seeded modes

`seed_mongo_orders` inserts three small orders, `seed_mongo_nested` one document with a nested object and an array (for the JSON tree). `seed_redis_types` puts one key of every type into db 0: two hashes under `user:`, a lookalike `username` key that a `user` group scan must not pick up, a JSON string with a TTL, a 150 element list so paging goes past one 100 item page, a set, a sorted set, a stream with a consumer group, and a non UTF-8 string. In `all` mode the redis contract tests use databases 1 to 3, so seeding db 0 for the browser suite doesn't step on them. Mongo seeds use a different database name per scenario for the same reason.

## browser tests

Playwright (`frontend/tests/`) doesn't run against vite alone, `npm run test:server` builds the frontend in test mode and then `go build -tags server` and runs the real Go backend in Wails server mode on `127.0.0.1:34115`, with an isolated demo database. So the browser tests exercise real query execution end to end. Specs that need Mongo or Redis read the same env vars as the Go tests and skip without them, which is why they are usually run through the script modes above.

```sh
cd frontend
npm run check        # svelte-check
npm run test:unit    # vitest, no database needed
npm test             # playwright, sqlite demo only unless the env vars are set
```

## dev databases (not for tests)

`dev/docker-compose.yml` starts four persistent, pre-seeded databases for manual testing. The data is deterministic and shaped like a real shop database, it's not a benchmark and it's not real customer data.

```sh
docker compose -f dev/docker-compose.yml up -d --wait
```

| Engine | Address | Credentials / database |
| --- | --- | --- |
| PostgreSQL | `127.0.0.1:5433` | `postgres` / `quarro`, database `quarro_demo`, schema `public` |
| MySQL | `127.0.0.1:3306` | `root` / `quarro`, database `quarro_demo` |
| MongoDB | `127.0.0.1:27017` | no auth, database `quarro_demo` |
| Redis | `127.0.0.1:6380` | no auth |

Postgres and MySQL each have 2,000 customers, 250 products, 5,000 orders, 15,000 order items and 10,000 audit events across 15 tables and 6 views, with real primary and foreign keys (so editing, the SQL preview, the schema diagram and visual joins all have something to work on), plus functions, procedures, triggers and engine specific indexes (JSONB and partial/GIN on Postgres, JSON and compound on MySQL). Mongo has the same customers and products with 5,000 nested orders, validators, heterogeneous documents, two aggregation views and text indexes. Redis has the same data spread over hashes, JSON strings, sets, lists, sorted sets, streams, HyperLogLog, geo data and some expiring sessions.

The seed scripts in `dev/seed/` only run when a volume is empty. To throw the data away and reseed:

```sh
docker compose -f dev/docker-compose.yml down -v
docker compose -f dev/docker-compose.yml up -d --wait
```

`down` without `-v` keeps the data, `-v` deletes the volumes, only do that on these.
