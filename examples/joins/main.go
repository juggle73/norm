// Example joins queries across tables with norm's join builder.
//
// It shows two styles:
//   - explicit ON clauses via Inner/Left
//   - FK-driven Auto joins, where the ON clause is derived from `fk` tags
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

type User struct {
	Id   int64  `norm:"pk,dbType=bigserial"`
	Name string `norm:"notnull"`
}

type Order struct {
	Id int64 `norm:"pk,dbType=bigserial"`
	// fk names the referenced table (snake_case). Auto matches it against
	// registered table names, so it points at "users", not the model name.
	UserId int64 `norm:"fk=users,notnull"`
	Total  int   `norm:"notnull"`
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
	orm.AddModel(&User{}, "users")
	orm.AddModel(&Order{}, "orders")

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

	seed(ctx, pool, orm)

	// Explicit join with a hand-written ON clause.
	fmt.Println("# explicit Inner join")
	runJoin(ctx, pool, orm, func(u *norm.Model, o *norm.Model) *norm.Join {
		return norm.NewJoin(u).
			Inner(o, "orders.user_id = users.id").
			Where("orders.total >= ?", 50).
			Order("orders.total DESC")
	})

	// FK-driven join: the ON clause comes from the `fk` tag on Order.UserId.
	fmt.Println("\n# Auto join (ON derived from fk tag)")
	runJoin(ctx, pool, orm, func(u *norm.Model, o *norm.Model) *norm.Join {
		return norm.NewJoin(u).
			Auto(o).
			Order("orders.total DESC")
	})
}

// runJoin builds a join with the given builder, runs it, and prints each row.
func runJoin(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm, build func(u, o *norm.Model) *norm.Join) {
	var user User
	var order Order
	mUser, _ := orm.M(&user)
	mOrder, _ := orm.M(&order)

	j := build(mUser, mOrder)
	query, args, err := j.Select()
	if err != nil {
		log.Fatalf("build join: %v", err)
	}
	fmt.Println("  SQL:", query)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		// Pointers() collects scan targets from every model in the join,
		// so user and order are both filled in on each iteration.
		if err := rows.Scan(j.Pointers()...); err != nil {
			log.Fatalf("scan: %v", err)
		}
		fmt.Printf("  %-6s order #%d total=%d\n", user.Name, order.Id, order.Total)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}
}

func seed(ctx context.Context, pool *pgxpool.Pool, orm *norm.Norm) {
	if _, err := pool.Exec(ctx, "TRUNCATE orders, users RESTART IDENTITY"); err != nil {
		log.Fatalf("truncate: %v", err)
	}

	users := []User{{Name: "Alice"}, {Name: "Bob"}}
	for i := range users {
		m, _ := orm.M(&users[i])
		query, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
		if err := pool.QueryRow(ctx, query, vals...).Scan(m.Pointer("Id")); err != nil {
			log.Fatalf("seed user: %v", err)
		}
	}

	orders := []Order{
		{UserId: users[0].Id, Total: 120},
		{UserId: users[0].Id, Total: 40},
		{UserId: users[1].Id, Total: 75},
	}
	for i := range orders {
		m, _ := orm.M(&orders[i])
		query, vals, _ := m.Insert(norm.Exclude("id"))
		if _, err := pool.Exec(ctx, query, vals...); err != nil {
			log.Fatalf("seed order: %v", err)
		}
	}
}
