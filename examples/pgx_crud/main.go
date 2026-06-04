// Example pgx_crud demonstrates a full CRUD cycle with norm over a pgx pool.
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
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx", used by migrate
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

type User struct {
	Id    int64  `norm:"pk,dbType=bigserial"` // bigserial → auto-incrementing id
	Name  string `norm:"notnull"`
	Email string `norm:"unique,notnull"`
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

	// migrate uses database/sql; the pgx stdlib driver registers itself as "pgx".
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

	// Start from a clean table so the example is repeatable.
	if _, err := pool.Exec(ctx, "TRUNCATE users RESTART IDENTITY"); err != nil {
		log.Fatalf("truncate: %v", err)
	}

	// CREATE — INSERT ... RETURNING id, scanned back into the struct.
	user := User{Name: "Alice", Email: "alice@example.com"}
	m, _ := orm.M(&user)
	query, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
	fmt.Println("INSERT:", query)
	if err := pool.QueryRow(ctx, query, vals...).Scan(m.Pointer("Id")); err != nil {
		log.Fatalf("insert: %v", err)
	}
	fmt.Printf("created user id=%d\n\n", user.Id)

	// READ — SELECT all columns into a fresh struct.
	var loaded User
	mr, _ := orm.M(&loaded)
	query, args, _ := mr.Select(norm.Where("id = ?", user.Id))
	fmt.Println("SELECT:", query)
	if err := pool.QueryRow(ctx, query, args...).Scan(mr.Pointers()...); err != nil {
		log.Fatalf("select: %v", err)
	}
	fmt.Printf("loaded: %+v\n\n", loaded)

	// UPDATE — SET clause built from the struct, WHERE chained on top.
	loaded.Name = "Alice Cooper"
	mu, _ := orm.M(&loaded)
	query, args, _ = mu.Update(norm.Exclude("id"), norm.Where("id = ?", loaded.Id))
	fmt.Println("UPDATE:", query)
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		log.Fatalf("update: %v", err)
	}
	fmt.Println("updated name -> Alice Cooper")
	fmt.Println()

	// DELETE.
	md, _ := orm.M(&User{})
	query, args, _ = md.Delete(norm.Where("id = ?", loaded.Id))
	fmt.Println("DELETE:", query)
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		log.Fatalf("delete: %v", err)
	}
	fmt.Println("deleted user")
}
