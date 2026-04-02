// Package norm is a lightweight SQL query builder for Go structs.
//
// It generates SQL fragments (field lists, bind parameters, WHERE conditions,
// full SELECT/INSERT/UPDATE/DELETE queries) from struct definitions using
// reflection. It is not an ORM — it does not execute queries or manage
// connections. You compose the generated SQL with any PostgreSQL driver (pgx,
// database/sql, etc.).
//
// # Quick start
//
//	type User struct {
//	    Id    int    `norm:"pk"`
//	    Name  string
//	    Email string
//	}
//
//	orm := norm.NewNorm(nil)
//	orm.AddModel(&User{}, "users")
//
//	// Sync tables (create/add columns)
//	mig := migrate.New(db, orm)
//	mig.Sync(ctx)
//
//	// SELECT
//	user := User{}
//	m, _ := orm.M(&user)
//	sql, args, _ := m.Select(norm.Where("id = ?", 42))
//	_ = pool.QueryRow(ctx, sql, args...).Scan(m.Pointers()...)
//
//	// INSERT
//	sql, vals, _ := m.Insert(norm.Exclude("id"))
//	_, _ = pool.Exec(ctx, sql, vals...)
//
// # Field and table naming
//
// All names are automatically converted to snake_case:
//
//	UserProfile → user_profile (table)
//	UserName    → user_name   (column)
//	CreatedAt   → created_at  (column)
//
// If your database column doesn't follow snake_case, override with dbName:
//
//	UserName string `norm:"dbName=username"` // → column "username"
//
// # Struct tags
//
// Fields are configured via the "norm" struct tag:
//
//	pk           — mark as primary key
//	notnull      — NOT NULL constraint
//	unique       — UNIQUE constraint
//	default=val  — DEFAULT value
//	dbName=name  — override column name
//	dbType=type  — override PostgreSQL type
//	fk=Model     — foreign key (accepts CamelCase, camelCase, snake_case)
//	-            — skip field entirely
//
// # Configuration
//
// [Config] controls default PostgreSQL types and JSON codec:
//
//	orm := norm.NewNorm(&norm.Config{
//	    DefaultString: "varchar",      // default: "text"
//	    DefaultTime:   "timestamp",    // default: "timestamptz"
//	    DefaultJSON:   "json",         // default: "jsonb"
//	    JSONMarshal:   sonic.Marshal,  // default: encoding/json
//	    JSONUnmarshal: sonic.Unmarshal,
//	})
//
// # Thread safety
//
// Struct metadata is cached and shared safely across goroutines.
// [Model] is not safe for concurrent use — each goroutine should call
// [Norm.M] to get its own Model instance.
//
// # JSON fields
//
// Struct and *struct fields (except time.Time) are automatically marshaled
// to JSON when writing ([Model.Values], [Model.Insert], [Model.Update]) and
// unmarshaled when reading ([Model.Pointers]).
package norm
