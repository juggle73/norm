# norm examples

Runnable, self-contained examples. Each directory is an independent Go module
that uses the local copy of norm via a `replace` directive, so you can run them
straight from this checkout.

Every example needs a PostgreSQL instance. Each directory ships its own
`docker-compose.yml` that starts Postgres on `localhost:5432` with database
`norm` (user/password `norm`/`norm`).

## Run any example

```shell
cd examples/<name>
docker compose up -d
go run .
```

When you're done:

```shell
docker compose down -v
```

The connection string defaults to
`postgres://norm:norm@localhost:5432/norm?sslmode=disable` and can be overridden
with the `DATABASE_URL` environment variable.

> All examples publish Postgres on the same host port `5432`. Run one example at
> a time, or `docker compose down` the previous one first.

## Examples

| Directory | Shows |
|-----------|-------|
| [`pgx_crud`](pgx_crud) | Full CRUD cycle (INSERT / SELECT / UPDATE / DELETE) over a pgx pool, with `migrate.Sync` to create the table. |
| [`upsert`](upsert) | `INSERT ... ON CONFLICT` — `DO NOTHING`, `DO UPDATE`, composite keys, `RETURNING`. |
| [`dynamic_filters`](dynamic_filters) | Building `WHERE` clauses at runtime from optional filters with `BuildConditions`. |
| [`joins`](joins) | Querying across tables with `NewJoin` and FK-driven `Auto` joins. |
| [`migration_sync`](migration_sync) | `migrate.Sync`, `Diff` and `CreateTableSQL` against a live database. |
| [`json_fields`](json_fields) | Struct and map fields stored as `jsonb`, plus `->>` JSON queries. |
