// Example json_fields stores struct and map fields as jsonb.
//
// norm marshals struct / *struct / map fields to JSON on write and unmarshals
// them on read automatically — no tags required. The column type defaults to
// jsonb (configurable via Config.DefaultJSON).
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

// Address is stored as a single jsonb column.
type Address struct {
	City   string `json:"city"`
	Street string `json:"street"`
}

type Profile struct {
	Id       int64          `norm:"pk,dbType=bigserial"`
	Name     string         `norm:"notnull"`
	Address  Address        // jsonb (struct)
	Metadata map[string]any // jsonb (map)
}

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://norm:norm@localhost:5432/norm?sslmode=disable"
}

func main() {
	ctx := context.Background()

	orm := norm.NewNorm(nil)
	orm.AddModel(&Profile{}, "profiles")

	db, err := sql.Open("pgx", dsn())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := migrate.New(db, orm).Sync(ctx); err != nil {
		log.Fatalf("sync schema: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		log.Fatalf("connect pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, "TRUNCATE profiles RESTART IDENTITY"); err != nil {
		log.Fatalf("truncate: %v", err)
	}

	// INSERT — Address and Metadata are marshaled to JSON automatically.
	profiles := []Profile{
		{
			Name:     "Alice",
			Address:  Address{City: "Moscow", Street: "Tverskaya"},
			Metadata: map[string]any{"tier": "gold", "verified": true},
		},
		{
			Name:     "Bob",
			Address:  Address{City: "Berlin", Street: "Unter den Linden"},
			Metadata: map[string]any{"tier": "silver", "verified": false},
		},
	}
	for i := range profiles {
		m, _ := orm.M(&profiles[i])
		query, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
		if err := pool.QueryRow(ctx, query, vals...).Scan(m.Pointer("Id")); err != nil {
			log.Fatalf("insert: %v", err)
		}
	}
	fmt.Println("inserted", len(profiles), "profiles")

	// SELECT — JSON columns are unmarshaled straight back into the struct.
	var loaded Profile
	mr, _ := orm.M(&loaded)
	query, args, _ := mr.Select(norm.Where("id = ?", profiles[0].Id))
	if err := pool.QueryRow(ctx, query, args...).Scan(mr.Pointers()...); err != nil {
		log.Fatalf("select: %v", err)
	}
	fmt.Printf("\nloaded #%d: name=%s city=%s tier=%v\n",
		loaded.Id, loaded.Name, loaded.Address.City, loaded.Metadata["tier"])

	// QUERY by a JSON key using the ->> operator. The key is quoted inside the
	// condition so it survives as a jsonb text accessor: address->>'city' = $1.
	var byCity Profile
	mc, _ := orm.M(&byCity)
	where, vals := mc.BuildConditions(norm.Eq("address->>'city'", "Berlin"))
	query = fmt.Sprintf("SELECT %s FROM %s WHERE %s", mc.Fields(), mc.Table(), where[0])
	fmt.Println("\nSQL:", query)
	if err := pool.QueryRow(ctx, query, vals...).Scan(mc.Pointers()...); err != nil {
		log.Fatalf("json query: %v", err)
	}
	fmt.Printf("found by city=Berlin: %s (%s)\n", byCity.Name, byCity.Address.Street)
}
