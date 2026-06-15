// Package migrate creates and alters database tables to match registered
// norm models. The target dialect is taken from the [norm.Config] — PostgreSQL
// (default), SQLite and MySQL are supported.
//
// Use [Sync] for safe development migrations (CREATE TABLE + ADD COLUMN only).
// Use [Diff] to generate a full SQL diff for review before applying to production.
//
// Dialect notes:
//   - SQLite: ALTER COLUMN (type / NOT NULL changes) is not available, so
//     [Diff] reports only added and dropped columns; ADD COLUMN cannot add
//     UNIQUE or NOT NULL-without-default columns.
//   - MySQL: column changes are rendered with MODIFY COLUMN; ADD COLUMN has no
//     IF NOT EXISTS. Introspection targets the connected database (DATABASE()).
//
// PRIMARY KEY auto-increment is not emitted automatically for any dialect —
// declare it via a dbType tag (e.g. `norm:"pk,dbType=serial"` for PostgreSQL,
// `norm:"pk,dbType=INT AUTO_INCREMENT"` for MySQL).
//
//	mig := migrate.New(db, orm)
//	mig.Sync(ctx)              // dev: create tables, add columns
//	sql, _ := mig.Diff(ctx)    // prod: review SQL before applying
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/juggle73/norm/v4"
)

// Migrate compares registered norm models against the database schema
// and generates or executes DDL statements to bring them in sync.
type Migrate struct {
	db     *sql.DB
	norm   *norm.Norm
	schema schemaDialect
}

// New creates a new Migrate instance.
// db may be nil if you only need SQL generation methods ([CreateTableSQL]).
func New(db *sql.DB, n *norm.Norm) *Migrate {
	return &Migrate{db: db, norm: n, schema: schemaFor(n.GetConfig().Dialect)}
}

// quote wraps a table or column identifier in the dialect's quoting
// characters when norm's [norm.Config.QuoteIdentifiers] is enabled; otherwise
// it returns the name unchanged.
func (m *Migrate) quote(name string) string {
	cfg := m.norm.GetConfig()
	if cfg.QuoteIdentifiers {
		return cfg.Dialect.QuoteIdentifier(name)
	}
	return name
}

// dbColumn represents an existing column in the database.
type dbColumn struct {
	name       string
	dataType   string
	isNullable bool
	isPK       bool
	isUnique   bool
	fkRef      string // referenced table name, empty if not FK
}

// ── Go → SQL type mapping ────────────────────────────────────────────────────

// columnType returns the column type for a field, using the configured
// dialect's scalar mapping.
// Priority: dbType tag > time > JSON > []byte > map/slice > string > scalar kind.
func (m *Migrate) columnType(f *norm.Field) string {
	if dbType, ok := f.Tag("dbType"); ok {
		return dbType
	}

	t := f.Type()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	cfg := m.norm.GetConfig()

	if t == reflect.TypeOf(time.Time{}) {
		return cfg.DefaultTime
	}

	if f.IsJSON() {
		return cfg.DefaultJSON
	}

	if t == reflect.TypeOf([]byte(nil)) {
		return m.schema.blobType()
	}

	if t.Kind() == reflect.Map || t.Kind() == reflect.Slice {
		return cfg.DefaultJSON
	}

	if t.Kind() == reflect.String {
		return cfg.DefaultString
	}

	if st, ok := m.schema.scalarType(t.Kind()); ok {
		return st
	}

	return cfg.DefaultString
}

// specFor builds a columnSpec for ALTER TABLE ADD COLUMN rendering.
func (m *Migrate) specFor(f *norm.Field) columnSpec {
	c := columnSpec{name: m.quote(f.DbName()), colType: m.columnType(f)}
	_, c.notNull = f.Tag("notnull")
	if def, ok := f.Tag("default"); ok {
		c.def = def
	}
	_, c.unique = f.Tag("unique")
	if fkTable, ok := f.Tag("fk"); ok {
		table := strcase.ToSnake(fkTable)
		if pk := m.resolvePK(table); pk != "" {
			c.fkTable = m.quote(table)
			c.fkPK = m.quote(pk)
		}
	}
	return c
}

// normalizeType maps PostgreSQL type aliases to a canonical form for comparison.
func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "int", "int4", "integer":
		return "integer"
	case "int2", "smallint":
		return "smallint"
	case "int8", "bigint":
		return "bigint"
	case "float4", "real":
		return "real"
	case "float8", "double precision":
		return "double precision"
	case "timestamptz", "timestamp with time zone":
		return "timestamptz"
	case "timestamp", "timestamp without time zone":
		return "timestamp"
	case "varchar", "character varying":
		return "varchar"
	case "char", "character":
		return "char"
	case "bool", "boolean":
		return "boolean"
	default:
		return t
	}
}

// resolvePK returns the first PK column name of a registered table.
func (m *Migrate) resolvePK(table string) string {
	fields := m.norm.FieldsByTable(table)
	for _, f := range fields {
		if _, ok := f.Tag("pk"); ok {
			return f.DbName()
		}
	}
	return ""
}

// ── SQL generation ──────────────────────────────────────────────────────────

// CreateTableSQL returns a CREATE TABLE IF NOT EXISTS statement for a
// registered table. Includes PRIMARY KEY, NOT NULL, UNIQUE, DEFAULT,
// and FOREIGN KEY constraints.
func (m *Migrate) CreateTableSQL(table string) string {
	fields := m.norm.FieldsByTable(table)
	if fields == nil {
		return ""
	}

	var cols []string
	var pks []string
	var fks []string

	for _, f := range fields {
		col := m.quote(f.DbName()) + " " + m.columnType(f)

		_, isPK := f.Tag("pk")
		_, notNull := f.Tag("notnull")

		if notNull || isPK {
			col += " NOT NULL"
		}

		if defVal, ok := f.Tag("default"); ok {
			col += " DEFAULT " + defVal
		}

		if _, ok := f.Tag("unique"); ok {
			col += " UNIQUE"
		}

		if isPK {
			pks = append(pks, m.quote(f.DbName()))
		}

		if fkTable, ok := f.Tag("fk"); ok {
			refTable := strcase.ToSnake(fkTable)
			refPK := m.resolvePK(refTable)
			if refPK != "" {
				fks = append(fks, fmt.Sprintf(
					"FOREIGN KEY (%s) REFERENCES %s(%s)",
					m.quote(f.DbName()), m.quote(refTable), m.quote(refPK),
				))
			}
		}

		cols = append(cols, col)
	}

	if len(pks) > 0 {
		cols = append(cols, "PRIMARY KEY ("+strings.Join(pks, ", ")+")")
	}
	cols = append(cols, fks...)

	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n    %s\n);",
		m.quote(table), strings.Join(cols, ",\n    "))
}

// addColumnSQL returns an ALTER TABLE ADD COLUMN statement, rendered for the
// configured dialect.
func (m *Migrate) addColumnSQL(table string, f *norm.Field) string {
	return m.schema.addColumn(m.quote(table), m.specFor(f))
}

// ── Sync ────────────────────────────────────────────────────────────────────

// Sync creates missing tables and adds missing columns.
// It never drops columns, changes types, or removes constraints — safe for
// use in development without risk of data loss.
func (m *Migrate) Sync(ctx context.Context) error {
	for _, table := range m.norm.Tables() {
		exists, err := m.schema.tableExists(ctx, m.db, table)
		if err != nil {
			return err
		}

		if !exists {
			sql := m.CreateTableSQL(table)
			if _, err := m.db.ExecContext(ctx, sql); err != nil {
				return fmt.Errorf("create table %s: %w", table, err)
			}
			continue
		}

		existingCols, err := m.schema.queryColumns(ctx, m.db, table)
		if err != nil {
			return err
		}

		existingSet := make(map[string]bool, len(existingCols))
		for _, col := range existingCols {
			existingSet[col.name] = true
		}

		fields := m.norm.FieldsByTable(table)
		for _, f := range fields {
			if !existingSet[f.DbName()] {
				sql := m.addColumnSQL(table, f)
				if _, err := m.db.ExecContext(ctx, sql); err != nil {
					return fmt.Errorf("add column %s.%s: %w", table, f.DbName(), err)
				}
			}
		}
	}
	return nil
}

// ── Diff ────────────────────────────────────────────────────────────────────

// Diff compares the database schema with registered models and returns
// SQL statements to bring the database in sync. Unlike [Sync], Diff also
// detects columns to drop, type mismatches, and constraint changes.
//
// Diff does NOT execute anything — it returns SQL for human review.
func (m *Migrate) Diff(ctx context.Context) (string, error) {
	var stmts []string

	for _, table := range m.norm.Tables() {
		exists, err := m.schema.tableExists(ctx, m.db, table)
		if err != nil {
			return "", err
		}

		fields := m.norm.FieldsByTable(table)

		if !exists {
			stmts = append(stmts, m.CreateTableSQL(table))
			continue
		}

		existingCols, err := m.schema.queryColumns(ctx, m.db, table)
		if err != nil {
			return "", err
		}

		existingMap := make(map[string]*dbColumn, len(existingCols))
		for i := range existingCols {
			existingMap[existingCols[i].name] = &existingCols[i]
		}

		expectedSet := make(map[string]bool, len(fields))

		for _, f := range fields {
			expectedSet[f.DbName()] = true

			existing, ok := existingMap[f.DbName()]
			if !ok {
				stmts = append(stmts, m.addColumnSQL(table, f))
				continue
			}

			// Type and NOT NULL changes require in-place column alteration,
			// which SQLite does not support (it needs a full table rebuild).
			// Skip them for such dialects.
			if !m.schema.supportsAlterColumn() {
				continue
			}

			_, wantNotNull := f.Tag("notnull")
			if _, wantPK := f.Tag("pk"); wantPK {
				wantNotNull = true
			}

			stmts = append(stmts, m.schema.alterColumn(
				m.quote(table), m.quote(f.DbName()), m.columnType(f), *existing, wantNotNull,
			)...)
		}

		// Columns in DB but not in struct → DROP
		if m.schema.supportsDropColumn() {
			for _, col := range existingCols {
				if !expectedSet[col.name] {
					stmts = append(stmts, fmt.Sprintf(
						"ALTER TABLE %s DROP COLUMN %s;",
						m.quote(table), m.quote(col.name),
					))
				}
			}
		}
	}

	return strings.Join(stmts, "\n"), nil
}
