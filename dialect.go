package norm

import (
	"fmt"
	"strconv"
	"strings"
)

// Dialect abstracts the SQL syntax differences between database backends so
// that the query builders in norm can target PostgreSQL, SQLite, MySQL, and
// others. A Dialect is set on [Config.Dialect]; the default is [PostgreSQL].
//
// Implementations must be safe for concurrent use — they are stateless and
// shared across all models of a [Norm] instance.
type Dialect interface {
	// Placeholder returns the bind placeholder for the n-th parameter (1-based).
	// Positional dialects return a numbered marker ("$1"); ordinal dialects
	// return a constant marker ("?") and ignore n.
	Placeholder(n int) string

	// SupportsReturning reports whether the dialect supports a RETURNING clause
	// on INSERT/UPDATE/DELETE.
	SupportsReturning() bool

	// BuildUpsert renders the conflict-resolution clause (including a leading
	// space) from already-resolved db column names. conflictCols is the
	// conflict target; updateCols are the columns to overwrite on conflict.
	// When doNothing is true the clause performs no update.
	BuildUpsert(conflictCols, updateCols []string, doNothing bool) (string, error)

	// QuoteIdentifier quotes a table or column identifier for the dialect.
	QuoteIdentifier(name string) string

	// NativeComposites reports whether the dialect's typical driver binds Go
	// composite values (maps and slices) to columns directly. PostgreSQL (via
	// pgx) encodes maps to jsonb and slices to arrays natively, so norm leaves
	// them untouched. SQLite and MySQL drivers cannot, so norm JSON-marshals
	// map and non-[]byte slice fields itself.
	NativeComposites() bool
}

// PostgreSQL is the default [Dialect]. It uses "$N" placeholders, supports
// RETURNING, and renders upserts as "ON CONFLICT (...) DO UPDATE SET
// col = EXCLUDED.col".
var PostgreSQL Dialect = postgresDialect{}

// SQLite targets SQLite. It uses "?" placeholders and shares PostgreSQL's
// "ON CONFLICT" upsert syntax. RETURNING is supported (SQLite ≥ 3.35).
var SQLite Dialect = sqliteDialect{}

// MySQL targets MySQL and MariaDB. It uses "?" placeholders, does not support
// RETURNING, and renders upserts as "ON DUPLICATE KEY UPDATE col = VALUES(col)".
//
// Note: MySQL's ON DUPLICATE KEY UPDATE fires on any unique-key conflict and
// ignores the conflict-target columns passed to OnConflict — they are accepted
// for API parity but not emitted.
var MySQL Dialect = mysqlDialect{}

type postgresDialect struct{}

func (postgresDialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }
func (postgresDialect) SupportsReturning() bool  { return true }
func (postgresDialect) NativeComposites() bool   { return true }
func (postgresDialect) QuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
func (postgresDialect) BuildUpsert(conflictCols, updateCols []string, doNothing bool) (string, error) {
	return onConflictClause(conflictCols, updateCols, doNothing)
}

type sqliteDialect struct{}

func (sqliteDialect) Placeholder(int) string  { return "?" }
func (sqliteDialect) SupportsReturning() bool { return true }
func (sqliteDialect) NativeComposites() bool  { return false }
func (sqliteDialect) QuoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
func (sqliteDialect) BuildUpsert(conflictCols, updateCols []string, doNothing bool) (string, error) {
	return onConflictClause(conflictCols, updateCols, doNothing)
}

type mysqlDialect struct{}

func (mysqlDialect) Placeholder(int) string  { return "?" }
func (mysqlDialect) SupportsReturning() bool { return false }
func (mysqlDialect) NativeComposites() bool  { return false }
func (mysqlDialect) QuoteIdentifier(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}
func (mysqlDialect) BuildUpsert(conflictCols, updateCols []string, doNothing bool) (string, error) {
	if doNothing {
		// MySQL has no DO NOTHING; emit a harmless self-assignment so the row
		// is left unchanged on conflict.
		col := conflictCols[0]
		return fmt.Sprintf(" ON DUPLICATE KEY UPDATE %s=%s", col, col), nil
	}
	sets := make([]string, len(updateCols))
	for i, col := range updateCols {
		sets[i] = fmt.Sprintf("%s = VALUES(%s)", col, col)
	}
	return " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", "), nil
}

// MariaDB targets MariaDB. It behaves like [MySQL] (the two are wire- and
// SQL-compatible for everything norm generates): "?" placeholders, no
// RETURNING, "ON DUPLICATE KEY UPDATE" upserts, backtick quoting.
var MariaDB Dialect = mariaDialect{}

// CockroachDB targets CockroachDB, which speaks the PostgreSQL wire protocol
// and SQL dialect. It behaves like [PostgreSQL].
var CockroachDB Dialect = cockroachDialect{}

// YugabyteDB targets YugabyteDB's PostgreSQL-compatible (YSQL) API. It behaves
// like [PostgreSQL].
var YugabyteDB Dialect = yugabyteDialect{}

// These compatible dialects embed a base dialect so they inherit its behavior
// while remaining distinct values (so `cfg.Dialect == MySQL` is false for
// MariaDB) and leaving room to override a method later.
type mariaDialect struct{ mysqlDialect }
type cockroachDialect struct{ postgresDialect }
type yugabyteDialect struct{ postgresDialect }

// IsMySQLFamily reports whether d is [MySQL] or a MySQL-compatible dialect
// (such as [MariaDB]). It drives the schema and default-type selection that
// those dialects share in the migrate and gen subpackages.
func IsMySQLFamily(d Dialect) bool {
	switch d.(type) {
	case mysqlDialect, mariaDialect:
		return true
	default:
		return false
	}
}

// defaultTypes returns the dialect-appropriate default column types for
// string, time.Time, and JSON fields when no dbType tag or Config override
// is given.
func defaultTypes(d Dialect) (str, tm, js string) {
	switch {
	case d == SQLite:
		return "TEXT", "TIMESTAMP", "TEXT"
	case IsMySQLFamily(d):
		return "varchar(255)", "datetime", "json"
	default: // PostgreSQL, CockroachDB, YugabyteDB
		return "text", "timestamptz", "jsonb"
	}
}

// onConflictClause renders the PostgreSQL/SQLite "ON CONFLICT (...) DO ..."
// clause shared by both dialects.
func onConflictClause(conflictCols, updateCols []string, doNothing bool) (string, error) {
	clause := fmt.Sprintf(" ON CONFLICT (%s)", strings.Join(conflictCols, ", "))
	if doNothing {
		return clause + " DO NOTHING", nil
	}
	sets := make([]string, len(updateCols))
	for i, col := range updateCols {
		sets[i] = fmt.Sprintf("%s = EXCLUDED.%s", col, col)
	}
	return clause + " DO UPDATE SET " + strings.Join(sets, ", "), nil
}
