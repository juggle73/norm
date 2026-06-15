// Example mysql drives norm against a live MySQL database.
//
// It highlights what differs from PostgreSQL on MySQL:
//   - Config.Dialect = norm.MySQL — "?" placeholders, MySQL DDL/types
//   - generated keys come from Result.LastInsertId (MySQL has no RETURNING)
//   - upserts render as INSERT ... ON DUPLICATE KEY UPDATE
//
// Run it with:
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

	_ "github.com/go-sql-driver/mysql"
	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

// Account uses an AUTO_INCREMENT primary key declared via the dbType tag, so
// the database assigns the id and we read it back with LastInsertId.
type Account struct {
	Id    int64  `norm:"pk,dbType=INT AUTO_INCREMENT"`
	Email string `norm:"unique,notnull"`
	Plan  string `norm:"notnull,default='free'"`
}

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "norm:norm@tcp(localhost:3306)/norm?parseTime=true"
}

func main() {
	ctx := context.Background()

	db, err := sql.Open("mysql", dsn())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	orm := norm.NewNorm(&norm.Config{Dialect: norm.MySQL})
	orm.AddModel(&Account{}, "account")

	// Clean slate so the example is repeatable.
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS account"); err != nil {
		log.Fatalf("drop: %v", err)
	}

	mig := migrate.New(db, orm)
	fmt.Println("# CreateTableSQL (MySQL types)")
	fmt.Println(mig.CreateTableSQL("account"))
	if err := mig.Sync(ctx); err != nil {
		log.Fatalf("sync: %v", err)
	}

	// INSERT — MySQL has no RETURNING, so read the generated id from the Result.
	acc := Account{Email: "ann@example.com", Plan: "pro"}
	m, _ := orm.M(&acc)
	insertSQL, vals, _ := m.Insert(norm.Exclude("id"))
	fmt.Printf("\n# INSERT (note the ? placeholders)\n%s\n", insertSQL)
	res, err := db.ExecContext(ctx, insertSQL, vals...)
	if err != nil {
		log.Fatalf("insert: %v", err)
	}
	acc.Id, _ = res.LastInsertId()
	fmt.Printf("LastInsertId() = %d\n", acc.Id)

	// UPSERT — INSERT ... ON DUPLICATE KEY UPDATE.
	dup := Account{Email: "ann@example.com", Plan: "enterprise"}
	m, _ = orm.M(&dup)
	upsertSQL, vals, _ := m.Insert(norm.Exclude("id"), norm.OnConflict("email").DoUpdate("plan"))
	fmt.Printf("\n# UPSERT\n%s\n", upsertSQL)
	if _, err := db.ExecContext(ctx, upsertSQL, vals...); err != nil {
		log.Fatalf("upsert: %v", err)
	}

	// SELECT it back through the builder's scan pointers.
	var got Account
	m, _ = orm.M(&got)
	selectSQL, args, _ := m.Select(norm.Where("email = ?", "ann@example.com"))
	if err := db.QueryRowContext(ctx, selectSQL, args...).Scan(m.Pointers()...); err != nil {
		log.Fatalf("select: %v", err)
	}
	fmt.Printf("\n# SELECT\n%s\n→ %+v\n", selectSQL, got)
}
