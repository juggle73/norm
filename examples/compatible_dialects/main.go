// Example compatible_dialects prints the SQL norm generates for the same model
// across every supported dialect. It needs no database — it only builds SQL.
//
// It shows that selecting a dialect is a one-line change, and that the
// compatible dialects reuse a base dialect's output: MariaDB matches MySQL,
// and CockroachDB / YugabyteDB match PostgreSQL.
//
//	go run .
package main

import (
	"fmt"

	"github.com/juggle73/norm/v4"
	"github.com/juggle73/norm/v4/migrate"
)

type Order struct {
	Id    int64  `norm:"pk"`
	Email string `norm:"unique,notnull"`
	Total int    `norm:"notnull"`
}

func main() {
	dialects := []struct {
		name string
		d    norm.Dialect
	}{
		{"PostgreSQL (default)", norm.PostgreSQL},
		{"CockroachDB", norm.CockroachDB},
		{"YugabyteDB", norm.YugabyteDB},
		{"SQLite", norm.SQLite},
		{"MySQL", norm.MySQL},
		{"MariaDB", norm.MariaDB},
	}

	for _, dl := range dialects {
		orm := norm.NewNorm(&norm.Config{Dialect: dl.d})
		orm.AddModel(&Order{}, "orders")
		mig := migrate.New(nil, orm)
		m, _ := orm.M(&Order{})

		insert, _, _ := m.Insert(norm.Exclude("id"))
		upsert, _, _ := m.Insert(norm.Exclude("id"), norm.OnConflict("email").DoUpdate("total"))

		fmt.Printf("══ %s ══\n", dl.name)
		fmt.Printf("  family:    IsMySQLFamily=%v\n", norm.IsMySQLFamily(dl.d))
		fmt.Printf("  CREATE:    %s\n", oneLine(mig.CreateTableSQL("orders")))
		fmt.Printf("  INSERT:    %s\n", insert)
		fmt.Printf("  UPSERT:    %s\n\n", upsert)
	}

	fmt.Println("Note: CockroachDB/YugabyteDB output matches PostgreSQL; MariaDB matches MySQL.")
}

// oneLine collapses the multi-line CREATE TABLE into a single line for compact
// side-by-side comparison.
func oneLine(s string) string {
	out := ""
	for _, r := range s {
		if r == '\n' || r == '\t' {
			r = ' '
		}
		if r == ' ' && len(out) > 0 && out[len(out)-1] == ' ' {
			continue
		}
		out += string(r)
	}
	return out
}
