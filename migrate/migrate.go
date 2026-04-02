// Package migrate creates and alters PostgreSQL tables to match registered
// norm models.
//
// Use [Sync] for safe development migrations (CREATE TABLE + ADD COLUMN only).
// Use [Diff] to generate a full SQL diff for review before applying to production.
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
	db   *sql.DB
	norm *norm.Norm
}

// New creates a new Migrate instance.
// db may be nil if you only need SQL generation methods ([CreateTableSQL]).
func New(db *sql.DB, n *norm.Norm) *Migrate {
	return &Migrate{db: db, norm: n}
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

// ── Go → PostgreSQL type mapping ────────────────────────────────────────────

var kindToPg = map[reflect.Kind]string{
	reflect.Int:     "integer",
	reflect.Int8:    "smallint",
	reflect.Int16:   "smallint",
	reflect.Int32:   "integer",
	reflect.Int64:   "bigint",
	reflect.Uint:    "integer",
	reflect.Uint8:   "smallint",
	reflect.Uint16:  "integer",
	reflect.Uint32:  "bigint",
	reflect.Uint64:  "bigint",
	reflect.Float32: "real",
	reflect.Float64: "double precision",
	reflect.Bool:    "boolean",
	reflect.String:  "text",
}

// pgType returns the PostgreSQL type for a field.
// Priority: dbType tag > IsJSON > kind mapping > "text".
func (m *Migrate) pgType(f *norm.Field) string {
	if dbType, ok := f.Tag("dbType"); ok {
		return dbType
	}

	t := f.Type()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == reflect.TypeOf(time.Time{}) {
		return "timestamptz"
	}

	if f.IsJSON() {
		return "jsonb"
	}

	if t == reflect.TypeOf([]byte(nil)) {
		return "bytea"
	}

	if t.Kind() == reflect.Map {
		return "jsonb"
	}

	if t.Kind() == reflect.Slice {
		return "jsonb"
	}

	if pgType, ok := kindToPg[t.Kind()]; ok {
		if t.Kind() == reflect.String {
			cfg := m.norm.GetConfig()
			if cfg != nil && cfg.DefaultString != "" {
				return cfg.DefaultString
			}
		}
		return pgType
	}

	return "text"
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
		col := f.DbName() + " " + m.pgType(f)

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
			pks = append(pks, f.DbName())
		}

		if fkTable, ok := f.Tag("fk"); ok {
			refTable := strcase.ToSnake(fkTable)
			refPK := m.resolvePK(refTable)
			if refPK != "" {
				fks = append(fks, fmt.Sprintf(
					"FOREIGN KEY (%s) REFERENCES %s(%s)",
					f.DbName(), refTable, refPK,
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
		table, strings.Join(cols, ",\n    "))
}

// addColumnSQL returns an ALTER TABLE ADD COLUMN statement.
func (m *Migrate) addColumnSQL(table string, f *norm.Field) string {
	col := f.DbName() + " " + m.pgType(f)

	_, notNull := f.Tag("notnull")
	if notNull {
		if defVal, ok := f.Tag("default"); ok {
			col += " NOT NULL DEFAULT " + defVal
		} else {
			col += " NOT NULL"
		}
	} else if defVal, ok := f.Tag("default"); ok {
		col += " DEFAULT " + defVal
	}

	if _, ok := f.Tag("unique"); ok {
		col += " UNIQUE"
	}

	if fkTable, ok := f.Tag("fk"); ok {
		refTable := strcase.ToSnake(fkTable)
		refPK := m.resolvePK(refTable)
		if refPK != "" {
			col += fmt.Sprintf(" REFERENCES %s(%s)", refTable, refPK)
		}
	}

	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s;", table, col)
}

// ── Sync ────────────────────────────────────────────────────────────────────

// Sync creates missing tables and adds missing columns.
// It never drops columns, changes types, or removes constraints — safe for
// use in development without risk of data loss.
func (m *Migrate) Sync(ctx context.Context) error {
	for _, table := range m.norm.Tables() {
		exists, err := m.tableExists(ctx, table)
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

		existingCols, err := m.queryColumns(ctx, table)
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
		exists, err := m.tableExists(ctx, table)
		if err != nil {
			return "", err
		}

		fields := m.norm.FieldsByTable(table)

		if !exists {
			stmts = append(stmts, m.CreateTableSQL(table))
			continue
		}

		existingCols, err := m.queryColumns(ctx, table)
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

			// Type mismatch
			expectedType := m.pgType(f)
			if normalizeType(existing.dataType) != normalizeType(expectedType) {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s TYPE %s;",
					table, f.DbName(), expectedType,
				))
			}

			// NOT NULL changes
			_, wantNotNull := f.Tag("notnull")
			_, wantPK := f.Tag("pk")
			if wantPK {
				wantNotNull = true
			}

			if wantNotNull && existing.isNullable {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
					table, f.DbName(),
				))
			} else if !wantNotNull && !existing.isNullable && !existing.isPK {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
					table, f.DbName(),
				))
			}
		}

		// Columns in DB but not in struct → DROP
		for _, col := range existingCols {
			if !expectedSet[col.name] {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s DROP COLUMN %s;",
					table, col.name,
				))
			}
		}
	}

	return strings.Join(stmts, "\n"), nil
}

// ── DB schema queries ───────────────────────────────────────────────────────

func (m *Migrate) tableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := m.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name=$1)",
		table,
	).Scan(&exists)
	return exists, err
}

func (m *Migrate) queryColumns(ctx context.Context, table string) ([]dbColumn, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable
		 FROM information_schema.columns
		 WHERE table_name=$1
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", table, err)
	}
	defer rows.Close()

	var cols []dbColumn
	for rows.Next() {
		var name, dataType, nullable string
		if err := rows.Scan(&name, &dataType, &nullable); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", table, err)
		}
		cols = append(cols, dbColumn{
			name:       name,
			dataType:   dataType,
			isNullable: nullable == "YES",
		})
	}

	// Merge PK info
	pkSet, err := m.queryConstraintColumns(ctx, table, "PRIMARY KEY")
	if err != nil {
		return nil, err
	}

	// Merge unique info
	uniqueSet, err := m.queryConstraintColumns(ctx, table, "UNIQUE")
	if err != nil {
		return nil, err
	}

	// Merge FK info
	fkMap, err := m.queryForeignKeys(ctx, table)
	if err != nil {
		return nil, err
	}

	for i := range cols {
		if pkSet[cols[i].name] {
			cols[i].isPK = true
			cols[i].isNullable = false // PK is always NOT NULL
		}
		if uniqueSet[cols[i].name] {
			cols[i].isUnique = true
		}
		if ref, ok := fkMap[cols[i].name]; ok {
			cols[i].fkRef = ref
		}
	}

	return cols, nil
}

func (m *Migrate) queryConstraintColumns(ctx context.Context, table, constraintType string) (map[string]bool, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT kcu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		     ON tc.constraint_name = kcu.constraint_name
		     AND tc.table_schema = kcu.table_schema
		 WHERE tc.constraint_type = $1
		     AND tc.table_name = $2`, constraintType, table)
	if err != nil {
		return nil, fmt.Errorf("query %s for %s: %w", constraintType, table, err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, fmt.Errorf("scan %s for %s: %w", constraintType, table, err)
		}
		result[col] = true
	}
	return result, nil
}

func (m *Migrate) queryForeignKeys(ctx context.Context, table string) (map[string]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT kcu.column_name, ccu.table_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		     ON tc.constraint_name = kcu.constraint_name
		     AND tc.table_schema = kcu.table_schema
		 JOIN information_schema.constraint_column_usage ccu
		     ON ccu.constraint_name = tc.constraint_name
		     AND ccu.table_schema = tc.table_schema
		 WHERE tc.constraint_type = 'FOREIGN KEY'
		     AND tc.table_name = $1`, table)
	if err != nil {
		return nil, fmt.Errorf("query FK for %s: %w", table, err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var col, refTable string
		if err := rows.Scan(&col, &refTable); err != nil {
			return nil, fmt.Errorf("scan FK for %s: %w", table, err)
		}
		result[col] = refTable
	}
	return result, nil
}
