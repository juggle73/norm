package gen

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/juggle73/norm/v4"
)

// schema encapsulates the dialect-specific parts of code generation: mapping a
// database column type to a Go type and reading the live schema.
type schema interface {
	// goType maps a database column type to a Go type descriptor.
	goType(dataType string) (goTypeInfo, bool)
	// listTables returns the names of the tables to generate from.
	listTables(ctx context.Context, db *sql.DB, schemaName string) ([]string, error)
	// queryColumns returns the column metadata for a table.
	queryColumns(ctx context.Context, db *sql.DB, table string) ([]Col, error)
}

// schemaFor selects the gen schema matching the configured norm dialect.
// Unknown dialects fall back to PostgreSQL.
func schemaFor(d norm.Dialect) schema {
	switch {
	case d == norm.SQLite:
		return sqliteGen{}
	case norm.IsMySQLFamily(d):
		return mysqlGen{}
	default: // PostgreSQL, CockroachDB, YugabyteDB
		return postgresGen{}
	}
}

// ── PostgreSQL ───────────────────────────────────────────────────────────────

type postgresGen struct{}

// goType maps a Postgres type to a Go type. For arrays it receives the
// udt_name (e.g. "_int4") supplied by queryColumns and resolves the element
// type, so int4[] becomes []int rather than a generic []string.
//
// Unknown scalar types (geometry, tsvector, custom/enum types, etc.) return
// false and are intentionally skipped by the generator rather than guessed —
// Postgres is strictly typed, so a wrong guess would be misleading. (SQLite/
// MySQL fall back to string because their type systems are affinity-based /
// fully enumerated.)
func (postgresGen) goType(dataType string) (goTypeInfo, bool) {
	lower := strings.ToLower(dataType)
	// Array udt_names are the element type prefixed with "_" (e.g. "_int4").
	if strings.HasPrefix(lower, "_") {
		if info, ok := pgArrayElem[lower[1:]]; ok {
			return info, true
		}
		return goTypeInfo{"[]string", "", true}, true // unknown element type
	}
	info, ok := typeMap[lower]
	return info, ok
}

// pgArrayElem maps a Postgres array element udt_name (without the leading "_")
// to the Go slice type for that array column.
var pgArrayElem = map[string]goTypeInfo{
	"int2":        {"[]int16", "", true},
	"int4":        {"[]int", "", true},
	"int8":        {"[]int64", "", true},
	"float4":      {"[]float32", "", true},
	"float8":      {"[]float64", "", true},
	"numeric":     {"[]float64", "", true},
	"bool":        {"[]bool", "", true},
	"text":        {"[]string", "", true},
	"varchar":     {"[]string", "", true},
	"bpchar":      {"[]string", "", true},
	"uuid":        {"[]string", "", true},
	"date":        {"[]time.Time", "time", true},
	"timestamp":   {"[]time.Time", "time", true},
	"timestamptz": {"[]time.Time", "time", true},
}

func (postgresGen) listTables(ctx context.Context, db *sql.DB, schemaName string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT tablename FROM pg_tables WHERE schemaname=$1", schemaName)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table names: %w", err)
	}
	return tables, nil
}

func (postgresGen) queryColumns(ctx context.Context, db *sql.DB, tableName string) ([]Col, error) {
	colRows, err := db.QueryContext(ctx,
		`SELECT column_name, is_nullable, data_type, udt_name
		 FROM information_schema.columns
		 WHERE table_name=$1
		 ORDER BY ordinal_position`, tableName)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", tableName, err)
	}
	defer colRows.Close()

	var cols []Col
	for colRows.Next() {
		var name, nullable, dataType, udtName string
		if err := colRows.Scan(&name, &nullable, &dataType, &udtName); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", tableName, err)
		}
		// For arrays, data_type is the generic "ARRAY"; the element type is in
		// udt_name (e.g. "_int4"), which goType resolves to a precise slice.
		if dataType == "ARRAY" {
			dataType = udtName
		}
		cols = append(cols, Col{
			Name:       name,
			IsNullable: nullable == "YES",
			DataType:   dataType,
		})
	}
	if err := colRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate column for %s: %w", tableName, err)
	}

	pkSet, err := pgConstraintColumns(ctx, db, tableName, "PRIMARY KEY")
	if err != nil {
		return nil, err
	}
	uniqueSet, err := pgConstraintColumns(ctx, db, tableName, "UNIQUE")
	if err != nil {
		return nil, err
	}
	fkMap, err := pgForeignKeys(ctx, db, tableName)
	if err != nil {
		return nil, err
	}

	for i := range cols {
		if pkSet[cols[i].Name] {
			cols[i].IsPK = true
			cols[i].IsNullable = false
		}
		if uniqueSet[cols[i].Name] {
			cols[i].IsUnique = true
		}
		if ref, ok := fkMap[cols[i].Name]; ok {
			cols[i].FK = ref
		}
	}

	return cols, nil
}

func pgConstraintColumns(ctx context.Context, db *sql.DB, tableName, constraintType string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT kcu.column_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		     ON tc.constraint_name = kcu.constraint_name
		     AND tc.table_schema = kcu.table_schema
		 WHERE tc.constraint_type = $1
		     AND tc.table_name = $2`, constraintType, tableName)
	if err != nil {
		return nil, fmt.Errorf("query %s for %s: %w", constraintType, tableName, err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, fmt.Errorf("scan %s for %s: %w", constraintType, tableName, err)
		}
		result[col] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s for %s: %w", constraintType, tableName, err)
	}
	return result, nil
}

func pgForeignKeys(ctx context.Context, db *sql.DB, tableName string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT kcu.column_name, ccu.table_name
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.key_column_usage kcu
		     ON tc.constraint_name = kcu.constraint_name
		     AND tc.table_schema = kcu.table_schema
		 JOIN information_schema.constraint_column_usage ccu
		     ON ccu.constraint_name = tc.constraint_name
		     AND ccu.table_schema = tc.table_schema
		 WHERE tc.constraint_type = 'FOREIGN KEY'
		     AND tc.table_name = $1`, tableName)
	if err != nil {
		return nil, fmt.Errorf("query FK for %s: %w", tableName, err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var col, refTable string
		if err := rows.Scan(&col, &refTable); err != nil {
			return nil, fmt.Errorf("scan FK for %s: %w", tableName, err)
		}
		result[col] = refTable
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FK for %s: %w", tableName, err)
	}
	return result, nil
}

// ── SQLite ───────────────────────────────────────────────────────────────────

type sqliteGen struct{}

// goType maps a SQLite declared type to a Go type using type affinity rules,
// so user-declared variants (VARCHAR(255), DATETIME, etc.) are handled.
func (sqliteGen) goType(dataType string) (goTypeInfo, bool) {
	t := strings.ToLower(strings.TrimSpace(dataType))
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	switch {
	case strings.Contains(t, "int"):
		return goTypeInfo{"int64", "", false}, true
	case strings.Contains(t, "char"), strings.Contains(t, "clob"), strings.Contains(t, "text"):
		return goTypeInfo{"string", "", false}, true
	case strings.Contains(t, "blob"), t == "":
		return goTypeInfo{"[]byte", "", true}, true
	case strings.Contains(t, "real"), strings.Contains(t, "floa"), strings.Contains(t, "doub"),
		strings.Contains(t, "num"), strings.Contains(t, "dec"):
		return goTypeInfo{"float64", "", false}, true
	case strings.Contains(t, "bool"):
		return goTypeInfo{"bool", "", false}, true
	case strings.Contains(t, "date"), strings.Contains(t, "time"):
		return goTypeInfo{"time.Time", "time", false}, true
	default:
		return goTypeInfo{"string", "", false}, true
	}
}

func (sqliteGen) listTables(ctx context.Context, db *sql.DB, _ string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table names: %w", err)
	}
	return tables, nil
}

func (sqliteGen) queryColumns(ctx context.Context, db *sql.DB, table string) ([]Col, error) {
	// table comes from listTables (a real sqlite_master name); PRAGMA does not
	// accept bind parameters, so it is interpolated.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma table_info for %s: %w", table, err)
	}
	defer rows.Close()

	var cols []Col
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info for %s: %w", table, err)
		}
		cols = append(cols, Col{
			Name:       name,
			IsNullable: notNull == 0 && pk == 0,
			DataType:   ctype,
			IsPK:       pk > 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table_info for %s: %w", table, err)
	}
	rows.Close()

	uniqueSet, err := sqliteUniqueColumns(ctx, db, table)
	if err != nil {
		return nil, err
	}
	fkMap, err := sqliteForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}
	for i := range cols {
		if uniqueSet[cols[i].Name] {
			cols[i].IsUnique = true
		}
		if ref, ok := fkMap[cols[i].Name]; ok {
			cols[i].FK = ref
		}
	}

	return cols, nil
}

func sqliteUniqueColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma index_list for %s: %w", table, err)
	}
	defer rows.Close()

	var uniqueIdx []string
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, fmt.Errorf("scan index_list for %s: %w", table, err)
		}
		if unique == 1 {
			uniqueIdx = append(uniqueIdx, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index_list for %s: %w", table, err)
	}
	rows.Close()

	result := make(map[string]bool)
	for _, idx := range uniqueIdx {
		irows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", idx))
		if err != nil {
			return nil, fmt.Errorf("pragma index_info for %s: %w", idx, err)
		}
		var colNames []string
		for irows.Next() {
			var seqno, cid int
			var cname string
			if err := irows.Scan(&seqno, &cid, &cname); err != nil {
				irows.Close()
				return nil, fmt.Errorf("scan index_info for %s: %w", idx, err)
			}
			colNames = append(colNames, cname)
		}
		if err := irows.Err(); err != nil {
			irows.Close()
			return nil, fmt.Errorf("iterate index_info for %s: %w", idx, err)
		}
		irows.Close()
		if len(colNames) == 1 {
			result[colNames[0]] = true
		}
	}
	return result, nil
}

func sqliteForeignKeys(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma foreign_key_list for %s: %w", table, err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var (
			id, seq            int
			refTable           string
			from, to           string
			onUpdate, onDelete string
			match              string
		)
		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, fmt.Errorf("scan foreign_key_list for %s: %w", table, err)
		}
		result[from] = refTable
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate foreign_key_list for %s: %w", table, err)
	}
	return result, nil
}

// ── MySQL ──────────────────────────────────────────────────────────────────────

type mysqlGen struct{}

// goType maps a MySQL declared column type (as returned in COLUMN_TYPE, which
// includes the display width, e.g. "int(11)", "tinyint(1)") to a Go type.
func (mysqlGen) goType(dataType string) (goTypeInfo, bool) {
	t := strings.ToLower(strings.TrimSpace(dataType))
	// tinyint(1) is the conventional MySQL boolean.
	if strings.HasPrefix(t, "tinyint(1)") {
		return goTypeInfo{"bool", "", false}, true
	}
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "unsigned"))
	switch t {
	case "bool", "boolean":
		return goTypeInfo{"bool", "", false}, true
	case "tinyint":
		return goTypeInfo{"int8", "", false}, true
	case "smallint":
		return goTypeInfo{"int16", "", false}, true
	case "mediumint", "int", "integer":
		return goTypeInfo{"int", "", false}, true
	case "bigint":
		return goTypeInfo{"int64", "", false}, true
	case "float":
		return goTypeInfo{"float32", "", false}, true
	case "double", "double precision", "real", "decimal", "numeric":
		return goTypeInfo{"float64", "", false}, true
	case "date", "datetime", "timestamp", "time", "year":
		return goTypeInfo{"time.Time", "time", false}, true
	case "json":
		return goTypeInfo{"map[string]any", "", true}, true
	case "blob", "tinyblob", "mediumblob", "longblob", "binary", "varbinary":
		return goTypeInfo{"[]byte", "", true}, true
	default: // char, varchar, text variants, enum, set, and anything else
		return goTypeInfo{"string", "", false}, true
	}
}

// listTables introspects the currently connected database (DATABASE());
// schemaName is ignored, matching MySQL's single-database connection model.
func (mysqlGen) listTables(ctx context.Context, db *sql.DB, _ string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'")
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table names: %w", err)
	}
	return tables, nil
}

func (mysqlGen) queryColumns(ctx context.Context, db *sql.DB, table string) ([]Col, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, column_type, is_nullable, column_key
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ?
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", table, err)
	}
	defer rows.Close()

	var cols []Col
	for rows.Next() {
		var name, dataType, nullable, key string
		if err := rows.Scan(&name, &dataType, &nullable, &key); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", table, err)
		}
		cols = append(cols, Col{
			Name:       name,
			IsNullable: nullable == "YES" && key != "PRI",
			DataType:   dataType,
			IsPK:       key == "PRI",
			IsUnique:   key == "UNI",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns for %s: %w", table, err)
	}

	fkMap, err := mysqlForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}
	for i := range cols {
		if ref, ok := fkMap[cols[i].Name]; ok {
			cols[i].FK = ref
		}
	}
	return cols, nil
}

func mysqlForeignKeys(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, referenced_table_name
		 FROM information_schema.key_column_usage
		 WHERE table_schema = DATABASE() AND table_name = ?
		     AND referenced_table_name IS NOT NULL`, table)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate FK for %s: %w", table, err)
	}
	return result, nil
}
