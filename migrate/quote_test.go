package migrate

import (
	"testing"

	"github.com/juggle73/norm/v4"
)

func newQuotedMigrate(d norm.Dialect, objs ...any) *Migrate {
	n := norm.NewNorm(&norm.Config{Dialect: d, QuoteIdentifiers: true})
	for _, obj := range objs {
		n.M(obj)
	}
	return New(nil, n)
}

func TestQuoteCreateTableSQL_Postgres(t *testing.T) {
	mig := newQuotedMigrate(norm.PostgreSQL, &User{}, &Order{})
	sql := mig.CreateTableSQL("order")

	for _, want := range []string{
		`CREATE TABLE IF NOT EXISTS "order"`,
		`"id" integer NOT NULL`,
		`"user_id" integer NOT NULL`,
		`PRIMARY KEY ("id")`,
		`FOREIGN KEY ("user_id") REFERENCES "user"("id")`,
	} {
		assertContains(t, sql, want)
	}
}

func TestQuoteCreateTableSQL_MySQLBackticks(t *testing.T) {
	mig := newQuotedMigrate(norm.MySQL, &User{})
	sql := mig.CreateTableSQL("user")

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `user`",
		"`id`",
		"`email`",
		"PRIMARY KEY (`id`)",
	} {
		assertContains(t, sql, want)
	}
}

func TestQuoteAddColumnSQL(t *testing.T) {
	mig := newQuotedMigrate(norm.PostgreSQL, &User{})
	sql := mig.addColumnSQL("user", fieldByName(t, mig, "user", "email"))
	assertContains(t, sql, `ALTER TABLE "user" ADD COLUMN IF NOT EXISTS "email"`)
}
