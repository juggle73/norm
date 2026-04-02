# norm - SQL query helper for Go structs

norm is a lightweight library that simplifies building SQL queries from Go structs when working with PostgreSQL drivers (pgx). It is **not an ORM** - it does not execute queries or manage connections. Instead, it generates SQL fragments (field lists, bind parameters, WHERE conditions) that you compose into queries yourself.

## Install

```shell
go get -u github.com/juggle73/norm/v3
```

## Quick start

```go
package main

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/juggle73/norm/v3"
)

type User struct {
    Id    int64  `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    pool, _ := pgxpool.New(context.Background(), "postgres://localhost/mydb")

    orm := norm.NewNorm(nil)

    // SELECT
    user := User{}
    m, _ := orm.M(&user)
    sql := fmt.Sprintf("SELECT %s FROM %s WHERE id=$1", m.Fields(), m.Table())

    _ = pool.QueryRow(context.Background(), sql, 42).Scan(m.Pointers()...)
    fmt.Println(user) // user is populated

    // INSERT
    newUser := User{Name: "Alice", Email: "alice@example.com"}
    m, _ = orm.M(&newUser)
    sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
        m.Table(),
        m.Fields(norm.Exclude("id")),
        m.Binds(norm.Exclude("id")),
    )
    _, _ = pool.Exec(context.Background(), sql, m.Values(norm.Exclude("id"))...)
}
```

## Core concepts

### Norm instance

`Norm` is the entry point. It caches struct metadata so reflection only happens once per type.

```go
orm := norm.NewNorm(nil) // default config
orm := norm.NewNorm(&norm.Config{DefaultString: "varchar"}) // custom default string type
```

### Model

`Model` is a lightweight wrapper that binds cached metadata to a specific struct instance. Each call to `M()` returns a new `Model` bound to the given pointer.

```go
user := User{Id: 1, Name: "John"}
m, err := orm.M(&user)
// m.Values()   → reads from &user
// m.Pointers() → points into &user
```

**Model is not safe for concurrent use.** Metadata caching is thread-safe, but each goroutine should get its own Model via `M()`.

### AddModel

Use `AddModel` to register a struct with a custom table name:

```go
orm.AddModel(&User{}, "app_users") // table name = "app_users"
```

Without `AddModel`, `M()` auto-generates the table name from the struct name in snake_case (`User` -> `user`, `UserProfile` -> `user_profile`).

## Struct tags

Fields are configured via the `norm` struct tag:

```go
type User struct {
    Id        int       `norm:"pk"`
    Email     string    `norm:"unique,notnull"`
    Name      string    `norm:"dbName=full_name"`
    Role      string    `norm:"notnull,default='user'"`
    Data      string    `norm:"dbType=jsonb"`
    Internal  string    `norm:"-"`          // skip this field
    CreatedAt time.Time                     // no tag = auto snake_case name
}
```

| Tag | Description |
|-----|-------------|
| `pk` | Mark as primary key (supports composite) |
| `unique` | Add UNIQUE constraint |
| `notnull` | Add NOT NULL constraint |
| `default=value` | Set DEFAULT value |
| `dbName=name` | Override column name (default: snake_case of field name) |
| `dbType=type` | Override PostgreSQL type |
| `-` | Skip field entirely |

## Embedded structs

norm supports Go struct embedding for sharing common fields:

```go
type BaseModel struct {
    Id        int       `norm:"pk"`
    CreatedAt time.Time
    UpdatedAt time.Time
}

type User struct {
    BaseModel
    Name  string
    Email string `norm:"unique"`
}

// m.Fields() → "id, created_at, updated_at, name, email"
```

Both value and pointer embeddings are supported (`BaseModel` and `*BaseModel`).

## Query building

### SELECT

```go
user := User{}
m, _ := orm.M(&user)

sql := fmt.Sprintf("SELECT %s FROM %s WHERE id=$1", m.Fields(), m.Table())
err := pool.QueryRow(ctx, sql, 42).Scan(m.Pointers()...)
```

### SELECT with options

```go
// Only specific fields
sql := fmt.Sprintf("SELECT %s FROM %s", m.Fields(norm.Fields("id,name")), m.Table())

// Exclude fields
sql := fmt.Sprintf("SELECT %s FROM %s", m.Fields(norm.Exclude("created_at,updated_at")), m.Table())

// Table prefix (for JOINs)
sql := fmt.Sprintf("SELECT %s FROM users u", m.Fields(norm.Prefix("u.")))
// → "SELECT u.id, u.name, u.email FROM users u"
```

### INSERT

```go
user := User{Name: "Alice", Email: "alice@example.com"}
m, _ := orm.M(&user)

sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
    m.Table(),
    m.Fields(norm.Exclude("id")),
    m.Binds(norm.Exclude("id")),
)
_, err := pool.Exec(ctx, sql, m.Values(norm.Exclude("id"))...)
```

### INSERT with RETURNING

```go
sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) %s",
    m.Table(),
    m.Fields(norm.Exclude("id")),
    m.Binds(norm.Exclude("id")),
    m.Returning("Id"),
)
// → "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id"

err := pool.QueryRow(ctx, sql, m.Values(norm.Exclude("id"))...).Scan(m.Pointer("Id"))
```

### UPDATE

```go
user := User{Id: 1, Name: "Bob", Email: "bob@new.com"}
m, _ := orm.M(&user)

set, nextBind := m.UpdateFields(norm.Exclude("id"))
where, _ := norm.Where("id = ?", user.Id).(*norm.WhereOptionAccessor)
whereStr, _ := where.Build(nextBind) // placeholder numbering continues from SET

sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s", m.Table(), set, whereStr)
// → "UPDATE users SET name=$1, email=$2 WHERE id=$3"

args := append(m.Values(norm.Exclude("id")), user.Id)
_, err := pool.Exec(ctx, sql, args...)
```

### ORDER BY

Field names are validated against the model and converted to database column names:

```go
m.OrderBy("Name ASC, Email DESC")
// → "name ASC, email DESC"

sql := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s",
    m.Fields(), m.Table(), m.OrderBy("Name DESC"))
```

Accepts field names in any format (struct name, camelCase, snake_case). Direction is optional (defaults to ASC). Panics on unknown fields or invalid direction.

### LIMIT / OFFSET

```go
sql := fmt.Sprintf("SELECT %s FROM %s %s",
    m.Fields(), m.Table(),
    m.LimitOffset(norm.Limit(10), norm.Offset(20)),
)
// → "SELECT id, name, email FROM users LIMIT 10 OFFSET 20"
```

### Extra scan targets

When your query returns columns not in the struct (e.g. computed columns):

```go
var totalCount int
ptrs := m.Pointers(norm.AddTargets(&totalCount))
// ptrs = [&user.Id, &user.Name, &user.Email, &totalCount]
```

## WHERE conditions builder

`BuildConditions` builds WHERE clauses from a map:

```go
m, _ := orm.M(&User{})

// Simple equality
conds, vals := m.BuildConditions(map[string]any{
    "name": "John",
}, "")
// conds = ["name=$1"], vals = ["John"]

// Comparison operators
conds, vals := m.BuildConditions(map[string]any{
    "age": map[string]any{"gte": 18, "lt": 65},
}, "")
// conds = ["age >= $1 AND age < $2"], vals = [18, 65]

// IN clause
conds, vals := m.BuildConditions(map[string]any{
    "name": []any{"Alice", "Bob"},
}, "")
// conds = ["name IN ($1, $2)"], vals = ["Alice", "Bob"]

// LIKE
conds, vals := m.BuildConditions(map[string]any{
    "name": map[string]any{"like": "%john%"},
}, "")
// conds = ["name LIKE $1"], vals = ["%john%"]

// IS NULL / IS NOT NULL
conds, vals := m.BuildConditions(map[string]any{
    "email": map[string]any{"isNull": true},
}, "")
// conds = ["email IS NULL"], vals = []

// With table prefix (for JOINs)
conds, vals := m.BuildConditions(map[string]any{
    "name": "John",
}, "u.")
// conds = ["u.name=$1"]

// JSON field access
conds, vals := m.BuildConditions(map[string]any{
    "data->>key": "value",
}, "")
// conds = ["data->>key=$1"]
```

### Supported condition operators

| Operator | Types | Example |
|----------|-------|---------|
| equality | string, int, uint, float, bool | `"name": "John"` |
| `gt`, `gte`, `lt`, `lte`, `ne` | int, uint, float, time | `"age": map[string]any{"gte": 18}` |
| `like` | string | `"name": map[string]any{"like": "%J%"}` |
| `isNull` | all | `"name": map[string]any{"isNull": true}` |
| IN | all (via slice) | `"id": []any{1, 2, 3}` |

## Migrations

Generate CREATE TABLE and ALTER TABLE statements from struct definitions:

```go
m, _ := orm.M(&User{})

// Generate CREATE TABLE
sql := m.CreateTableSQL()
// CREATE TABLE IF NOT EXISTS users (
//     id integer NOT NULL,
//     name text NOT NULL,
//     email text,
//     CONSTRAINT users_pkey PRIMARY KEY(id),
//     CONSTRAINT unique_users_email UNIQUE(email)
// )

// Generate ALTER TABLE for missing fields
// Pass existing column names from the database:
stmts := m.Migrate([]string{"id", "name"})
// → ["ALTER TABLE users ADD email text"]

// New table (nil or empty slice):
stmts := m.Migrate(nil)
// → [full CREATE TABLE statement]
```

### Type mappings

| Go type | PostgreSQL type |
|---------|----------------|
| `int` | integer |
| `int64` | bigint |
| `uint` | integer |
| `uint64` | bigint |
| `float32` | real |
| `float64` | double precision |
| `bool` | boolean |
| `string` | text (configurable via `Config.DefaultString`) |
| `time.Time` | timestamp with time zone |
| `pgtype.Timestamptz` | timestamp with time zone |
| `map[string]any` | json |
| `[]byte` | bytea |
| `[]string` | array |

Override with `dbType` tag: `norm:"dbType=jsonb"`.

## Code generation

Generate Go structs from an existing database schema:

```go
orm := norm.NewNorm(nil)
results, err := orm.GenFromDb(pool, "models", "public")
// results is map[tableName]string with generated Go source code

for tableName, source := range results {
    os.WriteFile(tableName+".go", []byte(source), 0644)
}
```

## MapScanner

For JOIN queries, `MapScanner` organizes results by source table:

```go
scanner, err := norm.NewMapScanner(pool)

rows, _ := pool.Query(ctx, "SELECT u.id, u.name, o.id, o.total FROM users u JOIN orders o ON ...")
for rows.Next() {
    result, _ := scanner.Scan(rows)
    // result = map[string]any{
    //     "users":  map[string]any{"id": 1, "name": "John"},
    //     "orders": map[string]any{"id": 42, "total": 99.99},
    // }
}
```

## Options reference

| Option | Description | Used by |
|--------|-------------|---------|
| `Exclude("field1,field2")` | Exclude fields by db name | Fields, Binds, UpdateFields, Pointers, Values |
| `Fields("field1,field2")` | Include only these fields | Fields, Binds, UpdateFields, Pointers, Values |
| `Prefix("t.")` | Add table alias prefix | Fields |
| `Returning("field1,field2")` | Fields for RETURNING clause | *(standalone option, see below)* |
| `Limit(n)` | LIMIT value | LimitOffset |
| `Offset(n)` | OFFSET value | LimitOffset |
| `AddTargets(&var1, &var2)` | Extra scan targets | Pointers |
| `Where("field = ?", val)` | WHERE with ? placeholders | ComposeOptions |

## Model methods reference

| Method | Returns | Description |
|--------|---------|-------------|
| `Fields(opts...)` | `string` | Comma-separated column names |
| `Binds(opts...)` | `string` | Bind placeholders `$1, $2, ...` |
| `UpdateFields(opts...)` | `string, int` | SET clause + next bind number |
| `Pointers(opts...)` | `[]any` | Field pointers for Scan |
| `Values(opts...)` | `[]any` | Field values for Exec |
| `Pointer(name)` | `any` | Single field pointer |
| `Table()` | `string` | Table name |
| `OrderBy(s)` | `string` | Validated ORDER BY clause |
| `Returning(fields)` | `string` | RETURNING clause |
| `LimitOffset(opts...)` | `string` | LIMIT/OFFSET clause |
| `BuildConditions(m, prefix)` | `[]string, []any` | WHERE conditions from map |
| `CreateTableSQL()` | `string` | CREATE TABLE statement |
| `Migrate(existing)` | `[]string` | ALTER TABLE statements |
| `FieldByName(name)` | `*Field, bool` | Find field by any name format |
| `FieldDescriptions()` | `[]*Field` | All field metadata |
| `NewInstance()` | `any` | New zero-value struct pointer |

## Important notes on v3

- `M(&obj)` returns a Model **bound to that specific instance**. `Pointers()`, `Values()`, and `Pointer()` work with the bound instance without additional parameters.
- Model is **not safe for concurrent use**. Each goroutine should call `M()` to get its own Model.
- Struct metadata is cached and shared safely across goroutines.
- `M()` returns `(*Model, error)`. Invalid argument (not a pointer to struct) returns an error.
- `OrderBy()`, `Returning()`, and `Pointer()` panic on unknown field names - these are programmer errors.
