package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/juggle73/norm/v4"
)

// columnSpec describes a single column for ALTER TABLE ADD COLUMN rendering.
type columnSpec struct {
	name    string
	colType string
	notNull bool
	def     string // default expression, "" if none
	unique  bool
	fkTable string // referenced table, "" if not a FK
	fkPK    string // referenced PK column, "" if unknown
}

// schemaDialect encapsulates the dialect-specific parts of schema generation
// and introspection: scalar type mapping, type normalization, ADD COLUMN
// rendering, the set of supported ALTER operations, and reading the live
// schema from the database.
type schemaDialect interface {
	// scalarType maps a non-string Go reflect.Kind to a column type.
	scalarType(reflect.Kind) (string, bool)
	// blobType returns the column type for []byte fields.
	blobType() string
	// normalizeType canonicalizes a type name for Diff comparison.
	normalizeType(string) string
	// addColumn renders a full "ALTER TABLE ... ADD COLUMN ..." statement.
	addColumn(table string, c columnSpec) string
	// supportsAlterColumn reports whether in-place column type / nullability
	// changes are available (false for SQLite).
	supportsAlterColumn() bool
	// supportsDropColumn reports whether DROP COLUMN is available.
	supportsDropColumn() bool

	// alterColumn returns the DDL statements that bring an existing column to
	// the desired type and nullability. qtable and qcol are already quoted by
	// the caller; newType is the raw target column type. Returns nil when no
	// change is needed. Only called when supportsAlterColumn is true.
	alterColumn(qtable, qcol, newType string, existing dbColumn, wantNotNull bool) []string

	tableExists(ctx context.Context, db *sql.DB, table string) (bool, error)
	queryColumns(ctx context.Context, db *sql.DB, table string) ([]dbColumn, error)
}

// schemaFor selects the schema dialect matching the configured norm dialect.
// Unknown dialects fall back to PostgreSQL.
func schemaFor(d norm.Dialect) schemaDialect {
	switch d {
	case norm.SQLite:
		return sqliteSchema{}
	case norm.MySQL:
		return mysqlSchema{}
	default:
		return postgresSchema{}
	}
}

// ── PostgreSQL ───────────────────────────────────────────────────────────────

type postgresSchema struct{}

var pgKind = map[reflect.Kind]string{
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
}

func (postgresSchema) scalarType(k reflect.Kind) (string, bool) {
	s, ok := pgKind[k]
	return s, ok
}

func (postgresSchema) blobType() string              { return "bytea" }
func (postgresSchema) normalizeType(t string) string { return normalizeType(t) }
func (postgresSchema) supportsAlterColumn() bool     { return true }
func (postgresSchema) supportsDropColumn() bool      { return true }

func (postgresSchema) addColumn(table string, c columnSpec) string {
	col := c.name + " " + c.colType
	if c.notNull {
		if c.def != "" {
			col += " NOT NULL DEFAULT " + c.def
		} else {
			col += " NOT NULL"
		}
	} else if c.def != "" {
		col += " DEFAULT " + c.def
	}
	if c.unique {
		col += " UNIQUE"
	}
	if c.fkTable != "" && c.fkPK != "" {
		col += fmt.Sprintf(" REFERENCES %s(%s)", c.fkTable, c.fkPK)
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s;", table, col)
}

func (postgresSchema) alterColumn(qtable, qcol, newType string, existing dbColumn, wantNotNull bool) []string {
	var stmts []string
	if normalizeType(existing.dataType) != normalizeType(newType) {
		stmts = append(stmts, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s;", qtable, qcol, newType))
	}
	if wantNotNull && existing.isNullable {
		stmts = append(stmts, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", qtable, qcol))
	} else if !wantNotNull && !existing.isNullable && !existing.isPK {
		stmts = append(stmts, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", qtable, qcol))
	}
	return stmts
}

func (postgresSchema) tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name=$1)",
		table,
	).Scan(&exists)
	return exists, err
}

func (postgresSchema) queryColumns(ctx context.Context, db *sql.DB, table string) ([]dbColumn, error) {
	rows, err := db.QueryContext(ctx,
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

	pkSet, err := pgConstraintColumns(ctx, db, table, "PRIMARY KEY")
	if err != nil {
		return nil, err
	}
	uniqueSet, err := pgConstraintColumns(ctx, db, table, "UNIQUE")
	if err != nil {
		return nil, err
	}
	fkMap, err := pgForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}

	for i := range cols {
		if pkSet[cols[i].name] {
			cols[i].isPK = true
			cols[i].isNullable = false
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

func pgConstraintColumns(ctx context.Context, db *sql.DB, table, constraintType string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx,
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

func pgForeignKeys(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
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

// ── SQLite ───────────────────────────────────────────────────────────────────

type sqliteSchema struct{}

var sqliteKind = map[reflect.Kind]string{
	reflect.Int:     "INTEGER",
	reflect.Int8:    "INTEGER",
	reflect.Int16:   "INTEGER",
	reflect.Int32:   "INTEGER",
	reflect.Int64:   "INTEGER",
	reflect.Uint:    "INTEGER",
	reflect.Uint8:   "INTEGER",
	reflect.Uint16:  "INTEGER",
	reflect.Uint32:  "INTEGER",
	reflect.Uint64:  "INTEGER",
	reflect.Float32: "REAL",
	reflect.Float64: "REAL",
	reflect.Bool:    "BOOLEAN",
}

func (sqliteSchema) scalarType(k reflect.Kind) (string, bool) {
	s, ok := sqliteKind[k]
	return s, ok
}

func (sqliteSchema) blobType() string          { return "BLOB" }
func (sqliteSchema) supportsAlterColumn() bool { return false }
func (sqliteSchema) supportsDropColumn() bool  { return true } // SQLite ≥ 3.35

// alterColumn is never invoked (supportsAlterColumn is false) — SQLite needs a
// full table rebuild for type/nullability changes.
func (sqliteSchema) alterColumn(_, _, _ string, _ dbColumn, _ bool) []string { return nil }

// normalizeType maps SQLite declared types to a canonical affinity-like form.
func (sqliteSchema) normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	// Strip any size/precision suffix, e.g. "varchar(255)".
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	switch {
	case strings.Contains(t, "int"):
		return "INTEGER"
	case strings.Contains(t, "char"), strings.Contains(t, "clob"), strings.Contains(t, "text"):
		return "TEXT"
	case strings.Contains(t, "blob"), t == "":
		return "BLOB"
	case strings.Contains(t, "real"), strings.Contains(t, "floa"), strings.Contains(t, "doub"):
		return "REAL"
	case strings.Contains(t, "bool"):
		return "BOOLEAN"
	case strings.Contains(t, "date"), strings.Contains(t, "time"):
		return "TIMESTAMP"
	default:
		return strings.ToUpper(t)
	}
}

func (sqliteSchema) addColumn(table string, c columnSpec) string {
	// SQLite ALTER TABLE ADD COLUMN has no IF NOT EXISTS, cannot add a UNIQUE
	// column, and only allows NOT NULL when a DEFAULT is provided.
	col := c.name + " " + c.colType
	switch {
	case c.notNull && c.def != "":
		col += " NOT NULL DEFAULT " + c.def
	case c.def != "":
		col += " DEFAULT " + c.def
	}
	if c.fkTable != "" && c.fkPK != "" {
		col += fmt.Sprintf(" REFERENCES %s(%s)", c.fkTable, c.fkPK)
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, col)
}

func (sqliteSchema) tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)",
		table,
	).Scan(&exists)
	return exists, err
}

func (sqliteSchema) queryColumns(ctx context.Context, db *sql.DB, table string) ([]dbColumn, error) {
	// table is a validated identifier ([a-zA-Z0-9_]); PRAGMA does not accept
	// bind parameters, so it is interpolated.
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma table_info for %s: %w", table, err)
	}
	defer rows.Close()

	var cols []dbColumn
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
		cols = append(cols, dbColumn{
			name:       name,
			dataType:   ctype,
			isNullable: notNull == 0 && pk == 0,
			isPK:       pk > 0,
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
		if uniqueSet[cols[i].name] {
			cols[i].isUnique = true
		}
		if ref, ok := fkMap[cols[i].name]; ok {
			cols[i].fkRef = ref
		}
	}

	return cols, nil
}

// sqliteUniqueColumns returns columns covered by a single-column UNIQUE index.
func sqliteUniqueColumns(ctx context.Context, db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma index_list for %s: %w", table, err)
	}
	defer rows.Close()

	type idx struct {
		name   string
		unique bool
	}
	var idxs []idx
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
			idxs = append(idxs, idx{name: name})
		}
	}
	rows.Close()

	result := make(map[string]bool)
	for _, ix := range idxs {
		irows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", ix.name))
		if err != nil {
			return nil, fmt.Errorf("pragma index_info for %s: %w", ix.name, err)
		}
		var colNames []string
		for irows.Next() {
			var seqno, cid int
			var cname string
			if err := irows.Scan(&seqno, &cid, &cname); err != nil {
				irows.Close()
				return nil, fmt.Errorf("scan index_info for %s: %w", ix.name, err)
			}
			colNames = append(colNames, cname)
		}
		irows.Close()
		// Only single-column unique indexes map to a UNIQUE column constraint.
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

// ── MySQL ─────────────────────────────────────────────────────────────────────

type mysqlSchema struct{}

var mysqlKind = map[reflect.Kind]string{
	reflect.Int:     "INT",
	reflect.Int8:    "TINYINT",
	reflect.Int16:   "SMALLINT",
	reflect.Int32:   "INT",
	reflect.Int64:   "BIGINT",
	reflect.Uint:    "INT UNSIGNED",
	reflect.Uint8:   "TINYINT UNSIGNED",
	reflect.Uint16:  "SMALLINT UNSIGNED",
	reflect.Uint32:  "INT UNSIGNED",
	reflect.Uint64:  "BIGINT UNSIGNED",
	reflect.Float32: "FLOAT",
	reflect.Float64: "DOUBLE",
	reflect.Bool:    "TINYINT(1)",
}

func (mysqlSchema) scalarType(k reflect.Kind) (string, bool) {
	s, ok := mysqlKind[k]
	return s, ok
}

func (mysqlSchema) blobType() string          { return "BLOB" }
func (mysqlSchema) supportsAlterColumn() bool { return true }
func (mysqlSchema) supportsDropColumn() bool  { return true }

// normalizeType canonicalizes a MySQL type for Diff comparison, stripping
// display width / length and the unsigned suffix.
func (mysqlSchema) normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "unsigned"))
	switch t {
	case "integer", "int", "int4":
		return "int"
	case "bool", "boolean", "tinyint":
		return "tinyint"
	case "double precision", "double", "float8":
		return "double"
	case "float", "real", "float4":
		return "float"
	case "character varying", "varchar", "character", "char":
		return "varchar"
	case "datetime", "timestamp":
		return "datetime"
	default:
		return t
	}
}

func (mysqlSchema) addColumn(table string, c columnSpec) string {
	// MySQL ADD COLUMN has no standard IF NOT EXISTS. Inline REFERENCES is
	// accepted for parity (MySQL parses but does not enforce a column-level
	// reference; a separate FOREIGN KEY constraint is needed to enforce it).
	col := c.name + " " + c.colType
	if c.notNull {
		if c.def != "" {
			col += " NOT NULL DEFAULT " + c.def
		} else {
			col += " NOT NULL"
		}
	} else if c.def != "" {
		col += " DEFAULT " + c.def
	}
	if c.unique {
		col += " UNIQUE"
	}
	if c.fkTable != "" && c.fkPK != "" {
		col += fmt.Sprintf(" REFERENCES %s(%s)", c.fkTable, c.fkPK)
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", table, col)
}

// alterColumn renders MySQL's MODIFY COLUMN, which restates the full column
// definition (type + nullability) in a single statement.
func (s mysqlSchema) alterColumn(qtable, qcol, newType string, existing dbColumn, wantNotNull bool) []string {
	typeChanged := s.normalizeType(existing.dataType) != s.normalizeType(newType)
	nullChanged := (wantNotNull && existing.isNullable) ||
		(!wantNotNull && !existing.isNullable && !existing.isPK)
	if !typeChanged && !nullChanged {
		return nil
	}
	def := newType
	if wantNotNull {
		def += " NOT NULL"
	}
	return []string{fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s;", qtable, qcol, def)}
}

func (mysqlSchema) tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)",
		table,
	).Scan(&exists)
	return exists, err
}

func (mysqlSchema) queryColumns(ctx context.Context, db *sql.DB, table string) ([]dbColumn, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, column_type, is_nullable, column_key
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ?
		 ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s: %w", table, err)
	}
	defer rows.Close()

	var cols []dbColumn
	for rows.Next() {
		var name, dataType, nullable, key string
		if err := rows.Scan(&name, &dataType, &nullable, &key); err != nil {
			return nil, fmt.Errorf("scan column for %s: %w", table, err)
		}
		cols = append(cols, dbColumn{
			name:       name,
			dataType:   dataType,
			isNullable: nullable == "YES" && key != "PRI",
			isPK:       key == "PRI",
			isUnique:   key == "UNI",
		})
	}

	fkMap, err := mysqlForeignKeys(ctx, db, table)
	if err != nil {
		return nil, err
	}
	for i := range cols {
		if ref, ok := fkMap[cols[i].name]; ok {
			cols[i].fkRef = ref
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
	return result, nil
}
