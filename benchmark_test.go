package norm

import (
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

// ─── norm setup ──────────────────────────────────────────────────────────────

var benchNorm = NewNorm(nil)

func benchModel() *Model {
	s := &ModelTestStruct{Id: 1, Name: "John", Email: "john@test.com", Age: 30}
	m, _ := benchNorm.M(s)
	return m
}

// ─── SELECT ──────────────────────────────────────────────────────────────────

func BenchmarkSelectNorm(b *testing.B) {
	m := benchModel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = m.Select(Where("id = ?", 1))
	}
}

func BenchmarkSelectSquirrel(b *testing.B) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = psql.Select("id", "name", "email", "age").
			From("model_test_struct").
			Where(sq.Eq{"id": 1}).
			ToSql()
	}
}

func BenchmarkSelectGoqu(b *testing.B) {
	dialect := goqu.Dialect("postgres")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = dialect.Select("id", "name", "email", "age").
			From("model_test_struct").
			Where(goqu.Ex{"id": 1}).
			ToSQL()
	}
}

// ─── INSERT ──────────────────────────────────────────────────────────────────

func BenchmarkInsertNorm(b *testing.B) {
	m := benchModel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = m.Insert(Exclude("id"))
	}
}

func BenchmarkInsertSquirrel(b *testing.B) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = psql.Insert("model_test_struct").
			Columns("name", "email", "age").
			Values("John", "john@test.com", 30).
			ToSql()
	}
}

func BenchmarkInsertGoqu(b *testing.B) {
	dialect := goqu.Dialect("postgres")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = dialect.Insert("model_test_struct").
			Cols("name", "email", "age").
			Vals(goqu.Vals{"John", "john@test.com", 30}).
			ToSQL()
	}
}

// ─── UPDATE ──────────────────────────────────────────────────────────────────

func BenchmarkUpdateNorm(b *testing.B) {
	m := benchModel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = m.Update(Exclude("id"), Where("id = ?", 1))
	}
}

func BenchmarkUpdateSquirrel(b *testing.B) {
	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = psql.Update("model_test_struct").
			Set("name", "John").
			Set("email", "john@test.com").
			Set("age", 30).
			Where(sq.Eq{"id": 1}).
			ToSql()
	}
}

func BenchmarkUpdateGoqu(b *testing.B) {
	dialect := goqu.Dialect("postgres")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = dialect.Update("model_test_struct").
			Set(goqu.Record{"name": "John", "email": "john@test.com", "age": 30}).
			Where(goqu.Ex{"id": 1}).
			ToSQL()
	}
}
