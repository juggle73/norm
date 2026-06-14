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
	switch d {
	case norm.SQLite:
		return sqliteGen{}
	default:
		return postgresGen{}
	}
}

// ── PostgreSQL ───────────────────────────────────────────────────────────────

type postgresGen struct{}

func (postgresGen) goType(dataType string) (goTypeInfo, bool) {
	info, ok := typeMap[strings.ToLower(dataType)]
	return info, ok
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
	return tables, nil
}

func (postgresGen) queryColumns(ctx context.Context, db *sql.DB, tableName string) ([]Col, error) {
	colRows, err := db.QueryContext(ctx,
		`SELECT column_name, is_nullable, data_type
		 FROM information_schema.columns
		 WHERE table_name=$1
		 ORDER BY ordinal_position`, tableName)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", tableName, err)
	}
	defer colRows.Close()

	var cols []Col
	for colRows.Next() {
		var name, nullable, dataType string
		if err := colRows.Scan(&name, &nullable, &dataType); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", tableName, err)
		}
		cols = append(cols, Col{
			Name:       name,
			IsNullable: nullable == "YES",
			DataType:   dataType,
		})
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
	return result, nil
}
