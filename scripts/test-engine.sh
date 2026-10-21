#!/usr/bin/env bash
# Disposable real database servers for contract/Playwright tests that need
# one, never the developer's own databases. One container set per mode,
# torn down on exit (including Ctrl-C) by the shared cleanup trap below.
#
# Usage: scripts/test-engine.sh <mode>
#   sql            PostgreSQL + MySQL with TLS; go test ./internal/...
#   mongo          MongoDB; internal/database/mongodb's contract test
#   mongo-explain  MongoDB seeded for the Explain Playwright scenario
#   mongo-chart    MongoDB seeded for the cross-engine chart Playwright scenario
#   mongo-json-tree MongoDB seeded with a nested document for the JSON
#                  tree viewer Playwright scenario
#   mongo-document-edit MongoDB seeded for the document insert/edit/delete
#                  Playwright scenario
#   mongo-native-query MongoDB seeded for the native aggregation-pipeline
#                  Playwright scenario
#   redis          Redis; internal/database/redis's contract test
#   redis-inspect  Redis seeded with one key of every type for the key
#                  inspector Playwright scenario
#   redis-key-edit Redis seeded the same way for the key editing
#                  Playwright scenario
#   redis-native-command Redis seeded the same way for the command editor
#                  Playwright scenario
#   mariadb        MariaDB; the ANALYZE FORMAT=JSON Explain path
#   ssh            PostgreSQL and MySQL, each reachable only through one
#                  disposable SSH jump host; the postgres/mysql packages'
#                  own SSH tunnel contract tests
#   all            Every server above, one go test ./internal/... pass
#                  covering all five engines' contract tests, then the full
#                  Playwright suite if QUARRO_BROWSER_TESTS=1
set -euo pipefail
cd "$(dirname "$0")/.."

containers=()
networks=()
work=""
cleanup() {
    for c in "${containers[@]}"; do docker rm -f "$c" >/dev/null 2>&1 || true; done
    for n in "${networks[@]}"; do docker network rm "$n" >/dev/null 2>&1 || true; done
    # A trap's own exit status overwrites the script's real one, so this must
    # never be the last statement to return non-zero (an empty $work, nothing
    # to clean up yet, would otherwise turn a clean "exit 2" usage error into
    # exit 1).
    if [[ -n "$work" ]]; then rm -rf "$work"; fi
    return 0
}
trap cleanup EXIT

wait_for() {
    # wait_for <description> <check command...>
    local desc=$1
    shift
    for _ in $(seq 1 120); do
        if "$@" >/dev/null 2>&1; then return 0; fi
        sleep 1
    done
    echo "timed out waiting for $desc" >&2
    return 1
}

start_mongo() {
    # Prints the container name; caller reads the mapped port separately, so
    # one container can be reused by more than one seed call in "all" mode.
    local name="quarro-mongo-$$-$RANDOM"
    containers+=("$name")
    docker run -d --name "$name" -p 127.0.0.1::27017 mongo:7 >/dev/null
    wait_for "MongoDB" docker exec "$name" mongosh --quiet --eval 'db.runCommand({ping:1})'
    echo "$name"
}

seed_mongo_orders() {
    # seed_mongo_orders <container> <database>. Both the Explain and the
    # cross-engine chart Playwright scenarios read the same small dataset
    # under whichever database name they default to (QUARRO_TEST_MONGO_DB
    # overrides that default; unset, each test picks its own).
    docker exec "$1" mongosh --quiet "$2" --eval '
    db.orders.insertMany([
      { status: "active", total: 10 },
      { status: "active", total: 5 },
      { status: "retired", total: 3 },
    ]);
  ' >/dev/null
}

seed_mongo_nested() {
    # seed_mongo_nested <container> <database>. One document with a nested
    # sub-document and an array field, for the JSON tree viewer Playwright
    # scenario to confirm actually renders/expands instead of the
    # [object Object]/comma-joined-array display bug it replaces.
    docker exec "$1" mongosh --quiet "$2" --eval '
    db.profiles.insertOne({
      name: "Ada",
      address: { city: "London", zip: "SW1" },
      tags: ["engineer", "writer"],
    });
  ' >/dev/null
}

start_redis() {
    local name="quarro-redis-$$-$RANDOM"
    containers+=("$name")
    docker run -d --name "$name" -p 127.0.0.1::6379 redis:7 >/dev/null
    wait_for "Redis" docker exec "$name" redis-cli ping
    echo "$name"
}

seed_redis_types() {
    # seed_redis_types <container>. One key of every type the key inspector
    # renders, in database 0: a hash group next to a lookalike "username"
    # key (the keyspace group scan must not include it), a JSON string with
    # a TTL, a 150-element list (more than one 100-element page), a set, a
    # sorted set, a stream with a consumer group, and a non-UTF-8 string.
    local cli=(docker exec -i "$1" redis-cli)
    "${cli[@]}" HSET user:1 name Ada plan pro >/dev/null
    "${cli[@]}" HSET user:2 name Grace plan free >/dev/null
    "${cli[@]}" SET username not-a-user-key >/dev/null
    "${cli[@]}" SET greeting '{"hello":"world","n":1}' EX 3600 >/dev/null
    seq -f 'item-%03g' 0 149 | xargs "${cli[@]}" RPUSH queue >/dev/null
    "${cli[@]}" SADD tags new featured >/dev/null
    "${cli[@]}" ZADD scores 1.5 ada 2 grace >/dev/null
    "${cli[@]}" XADD events 1-1 kind signup >/dev/null
    "${cli[@]}" XADD events 1-2 kind login >/dev/null
    "${cli[@]}" XGROUP CREATE events workers 0 >/dev/null
    printf '\xff\xfe\x00' | "${cli[@]}" -x SET blob >/dev/null
}

start_mariadb() {
    local name="quarro-mariadb-$$-$RANDOM"
    containers+=("$name")
    docker run -d --name "$name" -e MARIADB_ROOT_PASSWORD=quarro-contract -e MARIADB_DATABASE=quarro -p 127.0.0.1::3306 mariadb:11 >/dev/null
    # mariadb-admin ping can succeed slightly before root's grants are fully
    # applied by the image's own entrypoint; wait for a real authenticated
    # query to succeed, not just a ping.
    wait_for "MariaDB" docker exec "$name" mariadb -uroot -pquarro-contract -Nse 'SELECT 1'
    docker exec "$name" mariadb -uroot -pquarro-contract -Nse 'SELECT VERSION()' 2>/dev/null
    echo "$name"
}

# start_sql brings up PostgreSQL + MySQL with a shared private CA and
# exports QUARRO_TEST_POSTGRES_URL/SOCKET and QUARRO_TEST_MYSQL_URL/SOCKET.
start_sql() {
    work=$(mktemp -d)
    local prefix="quarro-sql-$$"
    containers+=("$prefix-pg" "$prefix-mysql")
    mkdir "$work/pg-socket" "$work/mysql-socket"
    chmod 777 "$work/pg-socket" "$work/mysql-socket"
    openssl req -x509 -newkey rsa:2048 -nodes -keyout "$work/ca.key" -out "$work/ca.pem" -days 1 -subj '/CN=Quarro Test CA' >/dev/null 2>&1
    openssl req -newkey rsa:2048 -nodes -keyout "$work/server.key" -out "$work/server.csr" -subj '/CN=localhost' >/dev/null 2>&1
    printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' >"$work/extensions"
    openssl x509 -req -in "$work/server.csr" -CA "$work/ca.pem" -CAkey "$work/ca.key" -CAcreateserial -out "$work/server.pem" -days 1 -extfile "$work/extensions" >/dev/null 2>&1
    # Copy certs into private container directories so ownership works on any host.
    docker run -d --name "$prefix-pg" -e POSTGRES_PASSWORD=quarro-contract -e POSTGRES_DB=quarro -p 127.0.0.1::5432 -v "$work/pg-socket:/var/run/postgresql" postgres:17 >/dev/null
    docker run -d --name "$prefix-mysql" -e MYSQL_ROOT_PASSWORD=quarro-contract -e MYSQL_DATABASE=quarro -p 127.0.0.1::3306 -v "$work/mysql-socket:/var/run/mysqld" mysql:8.4 >/dev/null
    for server in pg mysql; do
        docker exec "$prefix-$server" mkdir -p /certs
        for file in server.pem server.key ca.pem; do docker cp "$work/$file" "$prefix-$server:/certs/$file" >/dev/null; done
        owner=postgres
        [[ $server == mysql ]] && owner=mysql
        docker exec "$prefix-$server" chown -R "$owner:$owner" /certs
        docker exec "$prefix-$server" chmod 600 /certs/server.key
    done
    wait_for "PostgreSQL and MySQL" bash -c "docker exec $prefix-pg pg_isready -U postgres && docker exec $prefix-mysql mysqladmin ping -uroot -pquarro-contract"
    docker exec "$prefix-pg" psql -U postgres -d quarro -c "ALTER SYSTEM SET ssl = on" -c "ALTER SYSTEM SET ssl_cert_file = '/certs/server.pem'" -c "ALTER SYSTEM SET ssl_key_file = '/certs/server.key'" >/dev/null
    docker exec "$prefix-mysql" sh -c 'printf "[mysqld]\nssl-ca=/certs/ca.pem\nssl-cert=/certs/server.pem\nssl-key=/certs/server.key\n" > /etc/mysql/conf.d/quarro-tls.cnf'
    docker restart "$prefix-pg" "$prefix-mysql" >/dev/null
    wait_for "PostgreSQL and MySQL restart" bash -c "docker exec $prefix-pg pg_isready -U postgres && docker exec $prefix-mysql mysqladmin ping -uroot -pquarro-contract"
    local pgport myport
    pgport=$(docker port "$prefix-pg" 5432/tcp | cut -d: -f2)
    myport=$(docker port "$prefix-mysql" 3306/tcp | cut -d: -f2)
    export QUARRO_TEST_POSTGRES_SOCKET="$work/pg-socket"
    export QUARRO_TEST_MYSQL_SOCKET="$work/mysql-socket/mysqld.sock"
    export QUARRO_TEST_POSTGRES_URL="postgres://postgres:quarro-contract@127.0.0.1:$pgport/quarro?sslmode=verify-full&sslrootcert=$work/ca.pem"
    export QUARRO_TEST_MYSQL_URL="mysql://root:quarro-contract@127.0.0.1:$myport/quarro?sslmode=verify-full&sslrootcert=$work/ca.pem"
    docker exec "$prefix-pg" psql -U postgres -Atc 'SELECT version()'
    docker exec "$prefix-mysql" mysql -uroot -pquarro-contract -Nse 'SELECT VERSION()' 2>/dev/null
}

# start_ssh brings up PostgreSQL and MySQL on a private docker network with
# no published port (unreachable from the host directly) alongside one SSH
# jump host, on that same network, that does have a published port. A
# successful tunneled query therefore proves the tunnel, not an accidental
# direct route. Exports every QUARRO_TEST_SSH_* variable the postgres and
# mysql packages' own ssh_test.go files read.
start_ssh() {
    work=$(mktemp -d)
    local net="quarro-ssh-net-$$"
    networks+=("$net")
    docker network create "$net" >/dev/null
    local pg="quarro-ssh-pg-$$"
    local my="quarro-ssh-mysql-$$"
    local jump="quarro-ssh-jump-$$"
    containers+=("$pg" "$my" "$jump")
    docker run -d --name "$pg" --network "$net" --network-alias pgtarget \
        -e POSTGRES_PASSWORD=quarro-contract -e POSTGRES_DB=quarro postgres:17 >/dev/null
    docker run -d --name "$my" --network "$net" --network-alias mysqltarget \
        -e MYSQL_ROOT_PASSWORD=quarro-contract -e MYSQL_DATABASE=quarro mysql:8.4 >/dev/null
    docker run -d --name "$jump" --network "$net" -p 127.0.0.1::22 alpine:3 sleep infinity >/dev/null
    wait_for "PostgreSQL (SSH target)" docker exec "$pg" pg_isready -U postgres
    # A ping can succeed during the image's own internal init-then-restart
    # cycle, slightly before the real server is actually ready for queries
    # (the same race start_mariadb's own comment already documents); wait
    # for a real authenticated query instead.
    wait_for "MySQL (SSH target)" docker exec "$my" mysql -uroot -pquarro-contract -Nse 'SELECT 1'

    docker exec "$jump" sh -c 'apk add --no-cache openssh >/dev/null 2>&1 && ssh-keygen -A >/dev/null 2>&1'
    docker exec "$jump" sh -c 'adduser -D -s /bin/sh tunnel >/dev/null 2>&1 && echo "tunnel:tunnel-pass" | chpasswd'
    docker exec "$jump" sh -c 'mkdir -p /home/tunnel/.ssh && chmod 700 /home/tunnel/.ssh'
    # A client keypair generated on the host, for the key-auth case; its
    # public half is authorized inside the jump host, its private half is
    # handed to the Go test as QUARRO_TEST_SSH_KEY.
    ssh-keygen -t ed25519 -N '' -f "$work/client_key" -q
    docker cp "$work/client_key.pub" "$jump:/home/tunnel/.ssh/authorized_keys"
    docker exec "$jump" sh -c 'chown -R tunnel:tunnel /home/tunnel/.ssh && chmod 600 /home/tunnel/.ssh/authorized_keys'
    docker exec "$jump" sh -c "sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config"
    # Alpine's default sshd_config has an uncommented "AllowTcpForwarding
    # no"; OpenSSH takes a directive's first occurrence, so an appended
    # override is silently ignored and every tunnel dial fails with
    # "administratively prohibited" despite a correct login. Replace the
    # existing line in place instead.
    docker exec "$jump" sh -c "sed -i 's/AllowTcpForwarding no/AllowTcpForwarding yes/' /etc/ssh/sshd_config"
    docker exec -d "$jump" /usr/sbin/sshd -D
    wait_for "SSH jump host" docker exec "$jump" pgrep sshd

    local sshport
    sshport=$(docker port "$jump" 22/tcp | cut -d: -f2)
    export QUARRO_TEST_SSH_HOST=127.0.0.1
    export QUARRO_TEST_SSH_PORT="$sshport"
    export QUARRO_TEST_SSH_USER=tunnel
    export QUARRO_TEST_SSH_PASSWORD=tunnel-pass
    export QUARRO_TEST_SSH_KEY="$work/client_key"
    export QUARRO_TEST_SSH_TARGET_HOST=pgtarget
    export QUARRO_TEST_SSH_TARGET_PORT=5432
    export QUARRO_TEST_SSH_DB_USER=postgres
    export QUARRO_TEST_SSH_DB_PASSWORD=quarro-contract
    export QUARRO_TEST_SSH_DB_NAME=quarro
    export QUARRO_TEST_SSH_MYSQL_TARGET_HOST=mysqltarget
    export QUARRO_TEST_SSH_MYSQL_TARGET_PORT=3306
    export QUARRO_TEST_SSH_MYSQL_DB_USER=root
    export QUARRO_TEST_SSH_MYSQL_DB_PASSWORD=quarro-contract
    export QUARRO_TEST_SSH_MYSQL_DB_NAME=quarro
}

mode=${1:-}
case "$mode" in
    sql)
        start_sql
        go test -race -count=1 -v ./internal/...
        if [[ ${QUARRO_BROWSER_TESTS:-0} == 1 ]]; then (cd frontend && npm test); fi
        ;;
    mongo)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        go test -race -count=1 -v ./internal/database/mongodb/...
        ;;
    mongo-explain)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        seed_mongo_orders "$name" quarro_explain
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        export QUARRO_TEST_MONGO_DB="quarro_explain"
        (cd frontend && npm test -- -g "renders the same Graph View")
        ;;
    mongo-chart)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        seed_mongo_orders "$name" quarro_chart
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        export QUARRO_TEST_MONGO_DB="quarro_chart"
        (cd frontend && npm test -- -g "cross-engine")
        ;;
    mongo-json-tree)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        seed_mongo_nested "$name" quarro_json_tree
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        export QUARRO_TEST_MONGO_DB="quarro_json_tree"
        (cd frontend && npm test -- -g "nested JSON")
        ;;
    mongo-document-edit)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        seed_mongo_nested "$name" quarro_document_edit
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        export QUARRO_TEST_MONGO_DB="quarro_document_edit"
        (cd frontend && npm test -- -g "insert, edit, and delete a MongoDB document")
        ;;
    mongo-native-query)
        name=$(start_mongo)
        port=$(docker port "$name" 27017/tcp | cut -d: -f2)
        seed_mongo_orders "$name" quarro_native_query
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$port"
        export QUARRO_TEST_MONGO_DB="quarro_native_query"
        (cd frontend && npm test -- -g "native MongoDB aggregation pipeline")
        ;;
    redis)
        name=$(start_redis)
        port=$(docker port "$name" 6379/tcp | cut -d: -f2)
        export QUARRO_TEST_REDIS_URL="127.0.0.1:$port"
        go test -race -count=1 -v ./internal/database/redis/...
        ;;
    redis-inspect)
        name=$(start_redis)
        port=$(docker port "$name" 6379/tcp | cut -d: -f2)
        seed_redis_types "$name"
        export QUARRO_TEST_REDIS_URL="127.0.0.1:$port"
        (cd frontend && npm test -- -g "Redis key inspector")
        ;;
    redis-key-edit)
        name=$(start_redis)
        port=$(docker port "$name" 6379/tcp | cut -d: -f2)
        seed_redis_types "$name"
        export QUARRO_TEST_REDIS_URL="127.0.0.1:$port"
        (cd frontend && npm test -- -g "Redis key editing")
        ;;
    redis-native-command)
        name=$(start_redis)
        port=$(docker port "$name" 6379/tcp | cut -d: -f2)
        seed_redis_types "$name"
        export QUARRO_TEST_REDIS_URL="127.0.0.1:$port"
        (cd frontend && npm test -- -g "Redis command editor")
        ;;
    mariadb)
        name=$(start_mariadb)
        port=$(docker port "$name" 3306/tcp | cut -d: -f2)
        export QUARRO_TEST_MARIADB_URL="mysql://root:quarro-contract@127.0.0.1:$port/quarro?sslmode=disable"
        go test -race -count=1 -v ./internal/database/provider/... -run TestMariaDBExplainContract
        ;;
    ssh)
        start_ssh
        go test -race -count=1 -v ./internal/database/postgres/... ./internal/database/mysql/... -run TestSSHTunnelContract
        if [[ ${QUARRO_BROWSER_TESTS:-0} == 1 ]]; then (cd frontend && npm test -- -g "SSH tunnel"); fi
        ;;
    all)
        start_sql
        mongo_name=$(start_mongo)
        mongo_port=$(docker port "$mongo_name" 27017/tcp | cut -d: -f2)
        seed_mongo_orders "$mongo_name" quarro_explain
        seed_mongo_orders "$mongo_name" quarro_chart
        seed_mongo_nested "$mongo_name" quarro_json_tree
        seed_mongo_nested "$mongo_name" quarro_document_edit
        seed_mongo_orders "$mongo_name" quarro_native_query
        export QUARRO_TEST_MONGO_URL="127.0.0.1:$mongo_port"
        redis_name=$(start_redis)
        redis_port=$(docker port "$redis_name" 6379/tcp | cut -d: -f2)
        # The contract tests use databases 1-3, so seeding database 0 for
        # the browser suite does not collide with them.
        seed_redis_types "$redis_name"
        export QUARRO_TEST_REDIS_URL="127.0.0.1:$redis_port"
        mariadb_name=$(start_mariadb)
        mariadb_port=$(docker port "$mariadb_name" 3306/tcp | cut -d: -f2)
        export QUARRO_TEST_MARIADB_URL="mysql://root:quarro-contract@127.0.0.1:$mariadb_port/quarro?sslmode=disable"
        # Every engine's own env var is set, so this one pass exercises all five
        # engines' contract tests: each test package's own skip check (no
        # QUARRO_TEST_<ENGINE>_URL set) never triggers.
        go test -race -count=1 -v ./internal/...
        if [[ ${QUARRO_BROWSER_TESTS:-0} == 1 ]]; then (cd frontend && npm test); fi
        ;;
    *)
        echo "Usage: $0 <sql|mongo|mongo-explain|mongo-chart|mongo-json-tree|mongo-document-edit|mongo-native-query|redis|redis-inspect|redis-key-edit|redis-native-command|mariadb|ssh|all>" >&2
        exit 2
        ;;
esac
