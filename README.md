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

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/juggle73/norm/v3"
)

type User struct {
    Id    int64  `norm:"pk"`
    Name  string
    Email string
}

func main() {
    ctx := context.Background()
    pool, _ := pgxpool.New(ctx, "postgres://localhost/mydb")

    orm := norm.NewNorm(nil)

    // SELECT
    user := User{}
    m, _ := orm.M(&user)
    sql, args, _ := m.Select(norm.Where("id = ?", 42))
    // → "SELECT id, name, email FROM user WHERE id=$1"
    _ = pool.QueryRow(ctx, sql, args...).Scan(m.Pointers()...)

    // INSERT
    newUser := User{Name: "Alice", Email: "alice@example.com"}
    m, _ = orm.M(&newUser)
    sql, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
    // → "INSERT INTO user (name, email) VALUES ($1, $2) RETURNING id"
    _ = pool.QueryRow(ctx, sql, vals...).Scan(m.Pointer("Id"))

    // UPDATE
    user.Name = "Bob"
    m, _ = orm.M(&user)
    sql, args, _ = m.Update(norm.Exclude("id"), norm.Where("id = ?", user.Id))
    // → "UPDATE user SET name=$1, email=$2 WHERE id=$3"
    _, _ = pool.Exec(ctx, sql, args...)

    // DELETE
    sql, args, _ = m.Delete(norm.Where("id = ?", user.Id))
    // → "DELETE FROM user WHERE id=$1"
    _, _ = pool.Exec(ctx, sql, args...)
}
```

## Core concepts

### Norm instance

`Norm` is the entry point. It caches struct metadata so reflection only happens once per type.

```go
orm := norm.NewNorm(nil) // default config
orm := norm.NewNorm(&norm.Config{DefaultString: "varchar"}) // custom default string type

// Pluggable JSON codec (default: encoding/json)
orm := norm.NewNorm(&norm.Config{
    JSONMarshal:   sonic.Marshal,
    JSONUnmarshal: sonic.Unmarshal,
})
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
| `fk=ModelName` | Mark as foreign key (accepts any format: `UserType`, `userType`, `user_type`) |
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

## JSON struct fields

Fields of type struct or `*struct` (except `time.Time`) are automatically marshaled to JSON when writing and unmarshaled when reading. No tags required:

```go
type Address struct {
    City   string `json:"city"`
    Street string `json:"street"`
}

type User struct {
    Id      int     `norm:"pk"`
    Name    string
    Address Address // automatically handled as JSON
}

orm := norm.NewNorm(nil)

// INSERT — Address is marshaled to JSON bytes
user := User{Name: "Alice", Address: Address{City: "Moscow", Street: "Tverskaya"}}
m, _ := orm.M(&user)
sql, vals, _ := m.Insert(norm.Exclude("id"))
// vals = ["Alice", []byte(`{"city":"Moscow","street":"Tverskaya"}`)]

// SELECT — Address is unmarshaled from JSON
var loaded User
m, _ = orm.M(&loaded)
sql, args, _ = m.Select(norm.Where("id = ?", 1))
_ = pool.QueryRow(ctx, sql, args...).Scan(m.Pointers()...)
// loaded.Address.City == "Moscow"
```

Pointer struct fields (`*Address`) work the same way. `nil` pointers marshal to `null`.

By default `encoding/json` is used. For better performance, plug in a faster codec via `Config`:

```go
import "github.com/bytedance/sonic"

orm := norm.NewNorm(&norm.Config{
    JSONMarshal:   sonic.Marshal,
    JSONUnmarshal: sonic.Unmarshal,
})
```

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

### JOIN

Use `NewJoin` to build SELECT queries across multiple tables. Fields are auto-prefixed with table names, and `Pointers()` collects scan targets from all models:

```go
user := User{}
order := Order{}
mUser, _ := orm.M(&user)
mOrder, _ := orm.M(&order)

j := norm.NewJoin(mUser).
    Inner(mOrder, "orders.user_id = users.id").
    Where("users.active = ?", true).
    Order("users.name DESC").
    Limit(10)

sql, args, _ := j.Select()
// SELECT users.id, users.name, users.email, orders.id, orders.user_id, orders.total
//   FROM users
//   INNER JOIN orders ON orders.user_id = users.id
//   WHERE users.active=$1
//   ORDER BY users.name DESC LIMIT 10

err := pool.QueryRow(ctx, sql, args...).Scan(j.Pointers()...)
// user and order are populated
```

Multiple JOINs:

```go
item := OrderItem{}
mItem, _ := orm.M(&item)

j := norm.NewJoin(mUser).
    Inner(mOrder, "orders.user_id = users.id").
    Left(mItem, "order_items.order_id = orders.id").
    Where("users.id = ?", 1)

sql, args, _ := j.Select()
err := pool.QueryRow(ctx, sql, args...).Scan(j.Pointers()...)
```

Supported join types: `Inner`, `Left`, `Right`.

### Auto JOIN with FK tags

If your structs have `fk` tags, use `Auto` / `AutoLeft` to build JOINs without writing ON clauses:

```go
type User struct {
    Id   int    `norm:"pk"`
    Name string
}

type Order struct {
    Id     int `norm:"pk"`
    UserId int `norm:"fk=User"`
    Total  int
}

type OrderItem struct {
    Id      int    `norm:"pk"`
    OrderId int    `norm:"fk=Order"`
    Product string
}

mUser, _ := orm.M(&user)
mOrder, _ := orm.M(&order)
mItem, _ := orm.M(&item)

j := norm.NewJoin(mUser).
    Auto(mOrder).       // → INNER JOIN order ON order.user_id = user.id
    AutoLeft(mItem)     // → LEFT JOIN order_item ON order_item.order_id = order.id

sql, args, _ := j.Select()
err := pool.QueryRow(ctx, sql, args...).Scan(j.Pointers()...)
```

Auto works in both directions — it finds the FK regardless of which model defines it. Panics if the relationship is ambiguous (multiple FKs to the same table) or missing — use `Inner`/`Left`/`Right` in those cases.

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

Generate Go structs from an existing database schema. Code generation lives in a separate subpackage:

```go
import "github.com/juggle73/norm/v3/gen"

results, err := gen.FromDB(ctx, pool, "models", "public")
// results is map[tableName]string with generated Go source code

for tableName, source := range results {
    os.WriteFile(tableName+".go", []byte(source), 0644)
}
```

Generated structs include norm tags (`pk`, `notnull`, `unique`, `fk=...`) detected from database constraints.

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

## Join methods reference

| Method | Returns | Description |
|--------|---------|-------------|
| `NewJoin(base)` | `*Join` | Create join builder with FROM model |
| `Inner(m, on)` | `*Join` | Add INNER JOIN |
| `Left(m, on)` | `*Join` | Add LEFT JOIN |
| `Right(m, on)` | `*Join` | Add RIGHT JOIN |
| `Auto(m)` | `*Join` | INNER JOIN with ON from FK tags |
| `AutoLeft(m)` | `*Join` | LEFT JOIN with ON from FK tags |
| `Where(s, args...)` | `*Join` | Set WHERE clause |
| `Order(s)` | `*Join` | Set ORDER BY (raw SQL) |
| `Limit(n)` | `*Join` | Set LIMIT |
| `Offset(n)` | `*Join` | Set OFFSET |
| `Select()` | `string, []any, error` | Build SELECT query |
| `Pointers()` | `[]any` | Scan targets from all models |

## Important notes on v3

- `M(&obj)` returns a Model **bound to that specific instance**. `Pointers()`, `Values()`, and `Pointer()` work with the bound instance without additional parameters.
- Model is **not safe for concurrent use**. Each goroutine should call `M()` to get its own Model.
- Struct metadata is cached and shared safely across goroutines.
- `M()` returns `(*Model, error)`. Invalid argument (not a pointer to struct) returns an error.
- `OrderBy()`, `Returning()`, and `Pointer()` panic on unknown field names - these are programmer errors.
