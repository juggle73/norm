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

The simplest way — use `m.Select()`:

```go
user := User{}
m, _ := orm.M(&user)

sql, args, _ := m.Select(norm.Where("id = ?", 42))
// sql = "SELECT id, name, email FROM users WHERE id=$1"

err := pool.QueryRow(ctx, sql, args...).Scan(m.Pointers()...)
```

With all options:

```go
sql, args, _ := m.Select(
    norm.Exclude("password"),
    norm.Where("active = ?", true),
    norm.Order("Name DESC"),
    norm.Limit(10),
    norm.Offset(20),
)
// → "SELECT id, name, email FROM users WHERE active=$1 ORDER BY name DESC LIMIT 10 OFFSET 20"
```

You can also build SELECT manually with individual methods:

```go
sql := fmt.Sprintf("SELECT %s FROM %s WHERE id=$1", m.Fields(), m.Table())
err := pool.QueryRow(ctx, sql, 42).Scan(m.Pointers()...)
```

### INSERT

Use `m.Insert()` — returns SQL and values from the bound struct:

```go
user := User{Name: "Alice", Email: "alice@example.com"}
m, _ := orm.M(&user)

sql, vals, _ := m.Insert(norm.Exclude("id"))
// sql  = "INSERT INTO users (name, email) VALUES ($1, $2)"
// vals = ["Alice", "alice@example.com"]

_, err := pool.Exec(ctx, sql, vals...)
```

### INSERT with RETURNING

```go
sql, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
// → "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id"

err := pool.QueryRow(ctx, sql, vals...).Scan(m.Pointer("Id"))
```

### UPDATE

Use `m.Update()` — builds SET clause from bound struct values, chains bind numbers into WHERE:

```go
user := User{Id: 1, Name: "Bob", Email: "bob@new.com"}
m, _ := orm.M(&user)

sql, args, _ := m.Update(norm.Exclude("id"), norm.Where("id = ?", user.Id))
// sql  = "UPDATE users SET name=$1, email=$2 WHERE id=$3"
// args = ["Bob", "bob@new.com", 1]

_, err := pool.Exec(ctx, sql, args...)
```

With RETURNING:

```go
sql, args, _ := m.Update(
    norm.Exclude("id"),
    norm.Where("id = ?", user.Id),
    norm.Returning("Id"),
)
// → "UPDATE users SET name=$1, email=$2 WHERE id=$3 RETURNING id"
```

You can also build UPDATE manually with `UpdateFields` and `BuildWhere`:

```go
set, nextBind := m.UpdateFields(norm.Exclude("id"))
whereStr, whereArgs := norm.BuildWhere(nextBind, "id = ?", user.Id)

sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s", m.Table(), set, whereStr)
args := append(m.Values(norm.Exclude("id")), whereArgs...)
_, err := pool.Exec(ctx, sql, args...)
```

### DELETE

Use `m.Delete()`:

```go
m, _ := orm.M(&User{})

sql, args, _ := m.Delete(norm.Where("id = ?", 42))
// sql  = "DELETE FROM users WHERE id=$1"
// args = [42]

_, err := pool.Exec(ctx, sql, args...)
```

With RETURNING:

```go
sql, args, _ := m.Delete(norm.Where("id = ?", 42), norm.Returning("Id"))
// → "DELETE FROM users WHERE id=$1 RETURNING id"
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
    m.LimitOffset(10, 20),
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
| `Returning("field1,field2")` | Fields for RETURNING clause | Insert, Update, Delete |
| `Limit(n)` | LIMIT value | Select |
| `Offset(n)` | OFFSET value | Select |
| `Order("field [ASC\|DESC]")` | ORDER BY clause | Select |
| `AddTargets(&var1, &var2)` | Extra scan targets | Pointers |
| `Where("field = ?", val)` | WHERE with ? placeholders | Select, Update, Delete |

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
| `Select(opts...)` | `string, []any, error` | Full SELECT query + args |
| `Insert(opts...)` | `string, []any, error` | Full INSERT query + values |
| `Update(opts...)` | `string, []any, error` | Full UPDATE query + args |
| `Delete(opts...)` | `string, []any, error` | Full DELETE query + args |
| `Returning(fields)` | `string` | RETURNING clause |
| `LimitOffset(limit, offset)` | `string` | LIMIT/OFFSET clause |
| `BuildConditions(m, prefix)` | `[]string, []any` | WHERE conditions from map |
| `FieldByName(name)` | `*Field, bool` | Find field by any name format |
| `FieldDescriptions()` | `[]*Field` | All field metadata |
| `NewInstance()` | `any` | New zero-value struct pointer |

## Important notes on v3

- `M(&obj)` returns a Model **bound to that specific instance**. `Pointers()`, `Values()`, and `Pointer()` work with the bound instance without additional parameters.
- Model is **not safe for concurrent use**. Each goroutine should call `M()` to get its own Model.
- Struct metadata is cached and shared safely across goroutines.
- `M()` returns `(*Model, error)`. Invalid argument (not a pointer to struct) returns an error.
- `OrderBy()`, `Returning()`, and `Pointer()` panic on unknown field names - these are programmer errors.
