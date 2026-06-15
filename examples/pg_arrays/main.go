// Example pg_arrays shows how norm maps Go composite fields on PostgreSQL:
//   - a slice becomes a native array column ([]string -> text[], []int64 -> bigint[])
//   - a map and a struct field become jsonb
//   - []byte stays bytea
//
// migrate.Sync creates those column types; the round-trip uses a pgx pool
// (pgx's native interface), which binds and scans Postgres arrays and jsonb
// directly into Go slices and maps. (The database/sql array representation
// does not decode into a slice, so arrays need the native pgx interface.)
//
//	docker compose up -d
//	go run .
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

type Article struct {
	Id       int64          `norm:"pk,dbType=serial"`
	Title    string         `norm:"notnull"`
	Tags     []string       `norm:"notnull"` // -> text[]
	Versions []int64        `norm:"notnull"` // -> bigint[]
	Meta     Meta           `norm:"notnull"` // struct -> jsonb
	Extra    map[string]any // map -> jsonb
	Raw      []byte         // -> bytea
}

type Meta struct {
	Author string `json:"author"`
}

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://norm:norm@localhost:5432/norm?sslmode=disable"
}

func main() {
	ctx := context.Background()

	orm := norm.NewNorm(nil) // default dialect: PostgreSQL
	orm.AddModel(&Article{}, "article")

	// database/sql handle (pgx stdlib) for schema work.
	db, err := sql.Open("pgx", dsn())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS article"); err != nil {
		log.Fatalf("drop: %v", err)
	}

	mig := migrate.New(db, orm)
	fmt.Println("# CreateTableSQL — note text[] / bigint[] / jsonb / bytea")
	fmt.Println(mig.CreateTableSQL("article"))
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}
	printColumnTypes(ctx, db)

	// pgx pool (native interface) for the builder round-trip.
	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	in := Article{
		Title:    "Multi-dialect norm",
		Tags:     []string{"go", "sql", "orm"},
		Versions: []int64{1, 2, 3},
		Meta:     Meta{Author: "ann"},
		Extra:    map[string]any{"featured": true},
		Raw:      []byte{0xDE, 0xAD},
	}
	m, _ := orm.M(&in)
	insertSQL, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
	var id int64
	if err := pool.QueryRow(ctx, insertSQL, vals...).Scan(&id); err != nil {
		log.Fatalf("insert: %v", err)
	}
	fmt.Printf("\n# Inserted id=%d\n", id)

	var got Article
	m, _ = orm.M(&got)
	selectSQL, args, _ := m.Select(norm.Where("id = ?", id))
	if err := pool.QueryRow(ctx, selectSQL, args...).Scan(m.Pointers()...); err != nil {
		log.Fatalf("select: %v", err)
	}
	fmt.Println("\n# Round-trip through pgxpool:")
	fmt.Printf("  Tags     (text[])  = %v\n", got.Tags)
	fmt.Printf("  Versions (bigint[])= %v\n", got.Versions)
	fmt.Printf("  Meta     (jsonb)   = %+v\n", got.Meta)
	fmt.Printf("  Extra    (jsonb)   = %v\n", got.Extra)
	fmt.Printf("  Raw      (bytea)   = %v\n", got.Raw)
}

func printColumnTypes(ctx context.Context, db *sql.DB) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type FROM information_schema.columns
		 WHERE table_name='article' ORDER BY ordinal_position`)
	if err != nil {
		log.Fatalf("introspect: %v", err)
	}
	defer rows.Close()
	fmt.Println("\n# Column types in the database:")
	for rows.Next() {
		var name, dt string
		if err := rows.Scan(&name, &dt); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-10s %s\n", name, dt)
	}
}
