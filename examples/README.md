# norm examples

Runnable, self-contained examples. Each directory is an independent Go module
that uses the local copy of norm via a `replace` directive, so you can run them
straight from this checkout.

norm targets **PostgreSQL, SQLite and MySQL** (plus the compatible dialects
MariaDB, CockroachDB and YugabyteDB). The examples below cover all of them — see
the [multi-dialect examples](#multi-dialect) in particular.

## Running

Most examples need a database server and ship their own `docker-compose.yml`.
Two need nothing — [`sqlite`](sqlite) (pure-Go driver) and
[`compatible_dialects`](compatible_dialects) (only builds SQL) run on their own:

```shell
cd examples/<name>
go run .                 # sqlite / compatible_dialects
```

For the server-backed examples:

```shell
cd examples/<name>
docker compose up -d
go run .
docker compose down -v   # when done
```

The connection string can be overridden with the `DATABASE_URL` environment
variable. PostgreSQL examples publish port `5432` and MySQL port `3306`; run one
example per port at a time (or `docker compose down` the previous one).

## PostgreSQL examples

| Directory | Shows | Needs |
|-----------|-------|-------|
| [`pgx_crud`](pgx_crud) | Full CRUD cycle (INSERT / SELECT / UPDATE / DELETE) over a pgx pool, with `migrate.Sync`. | Postgres |
| [`upsert`](upsert) | `INSERT ... ON CONFLICT` — `DO NOTHING`, `DO UPDATE`, composite keys, `RETURNING`. | Postgres |
| [`dynamic_filters`](dynamic_filters) | Building `WHERE` clauses at runtime from optional filters with `BuildConditions`. | Postgres |
| [`joins`](joins) | Querying across tables with `NewJoin` and FK-driven `Auto` joins. | Postgres |
| [`migration_sync`](migration_sync) | `migrate.Sync`, `Diff` and `CreateTableSQL` against a live database. | Postgres |
| [`json_fields`](json_fields) | Struct and map fields stored as `jsonb`, plus `->>` JSON queries. | Postgres |
| [`pg_arrays`](pg_arrays) | Composite mapping on Postgres: slices → `text[]`/`bigint[]`, map/struct → `jsonb`, `[]byte` → `bytea`, round-tripped through a pgx pool. | Postgres |

## Multi-dialect

| Directory | Shows | Needs |
|-----------|-------|-------|
| [`sqlite`](sqlite) | `Dialect: norm.SQLite` — `migrate.Sync`, CRUD/UPSERT, RETURNING and `QuoteIdentifiers`. | nothing (pure-Go driver) |
| [`mysql`](mysql) | `Dialect: norm.MySQL` — `migrate.Sync`, CRUD, `LastInsertId` for generated keys (no RETURNING), `ON DUPLICATE KEY UPDATE`. | MySQL |
| [`compatible_dialects`](compatible_dialects) | The SQL norm generates for one model across all six dialects, side by side — MariaDB matches MySQL, CockroachDB/YugabyteDB match PostgreSQL. | nothing (only builds SQL) |
