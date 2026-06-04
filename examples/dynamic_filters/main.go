// Example dynamic_filters builds WHERE clauses at runtime from optional filters.
//
// The common pain point: a list endpoint where every filter is optional and you
// don't want to hand-concatenate SQL or juggle "$1, $2..." by hand. norm's
// BuildConditions turns a slice of typed conditions into a WHERE clause with
// correctly numbered placeholders.
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
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

type Product struct {
	Id       int64  `norm:"pk,dbType=bigserial"`
	Name     string `norm:"notnull"`
	Category string `norm:"notnull"`
	Price    int    `norm:"notnull"`
	InStock  bool   `norm:"notnull"`
}

// Filter holds the optional query parameters. Zero values mean "not set".
type Filter struct {
	Category    string
	MinPrice    int
	MaxPrice    int
	InStockOnly bool
}

// search builds and runs a SELECT whose WHERE clause depends on which filter
// fields are set.
func search(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm, f Filter) ([]Product, error) {
	var p Product
	m, _ := orm.M(&p)

	// Collect only the conditions that apply.
	var conds []norm.Cond
	if f.Category != "" {
		conds = append(conds, norm.Eq("category", f.Category))
	}
	if f.MinPrice > 0 {
		conds = append(conds, norm.Gte("price", f.MinPrice))
	}
	if f.MaxPrice > 0 {
		conds = append(conds, norm.Lte("price", f.MaxPrice))
	}
	if f.InStockOnly {
		conds = append(conds, norm.Eq("in_stock", true))
	}

	where, args := m.BuildConditions(conds...)

	query := fmt.Sprintf("SELECT %s FROM %s", m.Fields(), m.Table())
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY price"
	fmt.Println("  SQL:", query)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Product
	for rows.Next() {
		var item Product
		rm, _ := orm.M(&item)
		if err := rows.Scan(rm.Pointers()...); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
	orm.AddModel(&Product{}, "products")

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

	if _, err := pool.Exec(ctx, "TRUNCATE products RESTART IDENTITY"); err != nil {
		log.Fatalf("truncate: %v", err)
	}
	seed(ctx, pool, orm)

	cases := []struct {
		name   string
		filter Filter
	}{
		{"no filters", Filter{}},
		{"category=book", Filter{Category: "book"}},
		{"price 20..100, in stock", Filter{MinPrice: 20, MaxPrice: 100, InStockOnly: true}},
	}
	for _, c := range cases {
		fmt.Printf("\n# %s\n", c.name)
		found, err := search(ctx, pool, orm, c.filter)
		if err != nil {
			log.Fatalf("search: %v", err)
		}
		for _, p := range found {
			fmt.Printf("  - %-18s %-8s %4d  in_stock=%v\n", p.Name, p.Category, p.Price, p.InStock)
		}
	}
}

func seed(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm) {
	products := []Product{
		{Name: "Go in Action", Category: "book", Price: 35, InStock: true},
		{Name: "SQL Antipatterns", Category: "book", Price: 28, InStock: false},
		{Name: "Mechanical Keyboard", Category: "gadget", Price: 90, InStock: true},
		{Name: "USB-C Hub", Category: "gadget", Price: 45, InStock: true},
		{Name: "Standing Desk", Category: "furniture", Price: 300, InStock: false},
	}
	for i := range products {
		m, _ := orm.M(&products[i])
		query, vals, _ := m.Insert(norm.Exclude("id"))
		if _, err := pool.Exec(ctx, query, vals...); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}
}
