// Package gen generates Go struct source code from PostgreSQL database schemas.
//
// Usage:
//
//	results, err := gen.FromDB(ctx, pool, "models", "public")
//	for tableName, source := range results {
//	    os.WriteFile(tableName+".go", []byte(source), 0644)
//	}
package gen

import (
	"context"
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Col describes a database column for code generation.
type Col struct {
	Name       string
	IsNullable bool
	DataType   string
	IsPK       bool
	IsUnique   bool
	FK         string // referenced table name (empty if not FK)
}

type goTypeInfo struct {
	name       string
	importPath string
	isSlice    bool // slices and maps don't get pointer prefix for nullable
}

var typeMap = map[string]goTypeInfo{
	// Integers
	"smallint": {"int16", "", false},
	"integer":  {"int", "", false},
	"bigint":   {"int64", "", false},
	"serial":   {"int", "", false},
	"bigserial": {"int64", "", false},

	// Floats / Numeric
	"real":             {"float32", "", false},
	"float":            {"float32", "", false},
	"double precision": {"float64", "", false},
	"numeric":          {"float64", "", false},
	"decimal":          {"float64", "", false},
	"money":            {"string", "", false},

	// Strings
	"character varying": {"string", "", false},
	"character":         {"string", "", false},
	"text":              {"string", "", false},
	"uuid":              {"string", "", false},
	"inet":              {"string", "", false},
	"cidr":              {"string", "", false},
	"macaddr":           {"string", "", false},
	"interval":          {"string", "", false},
	"xml":               {"string", "", false},

	// Boolean
	"boolean": {"bool", "", false},

	// Date / Time
	"date":                        {"time.Time", "time", false},
	"time":                        {"time.Time", "time", false},
	"timetz":                      {"time.Time", "time", false},
	"time with time zone":         {"time.Time", "time", false},
	"time without time zone":      {"time.Time", "time", false},
	"timestamp":                   {"time.Time", "time", false},
	"timestamptz":                 {"time.Time", "time", false},
	"timestamp with time zone":    {"time.Time", "time", false},
	"timestamp without time zone": {"time.Time", "time", false},

	// JSON
	"json":  {"map[string]any", "", true},
	"jsonb": {"map[string]any", "", true},

	// Binary
	"bytea": {"[]byte", "", true},

	// Array
	"ARRAY": {"[]string", "", true},
}

// Gen generates Go struct source code from column definitions.
func Gen(packageName, structName string, cols []Col) string {
	imports := make(map[string]bool)
	structStr := fmt.Sprintf("type %s struct {\n", structName)

	for _, col := range cols {
		info, ok := typeMap[strings.ToLower(col.DataType)]
		if !ok {
			continue
		}

		pointerPrefix := ""
		if col.IsNullable && !info.isSlice {
			pointerPrefix = "*"
		}

		if info.importPath != "" {
			imports[info.importPath] = true
		}

		normTags := buildNormTags(col)
		normTagStr := ""
		if normTags != "" {
			normTagStr = fmt.Sprintf(` norm:"%s"`, normTags)
		}

		structStr += fmt.Sprintf("\t%s %s%s `json:\"%s\"%s`\n",
			strcase.ToCamel(col.Name),
			pointerPrefix,
			info.name,
			strcase.ToLowerCamel(col.Name),
			normTagStr,
		)
	}

	structStr += "}"

	res := fmt.Sprintf("package %s\n\n", packageName)
	if len(imports) > 0 {
		res += "import (\n"
		for k := range imports {
			res += fmt.Sprintf("\t\"%s\"\n", k)
		}
		res += ")\n\n"
	}

	res += structStr

	return res
}

// buildNormTags constructs the norm tag string for a column.
func buildNormTags(col Col) string {
	var tags []string

	if col.IsPK {
		tags = append(tags, "pk")
	}
	if !col.IsNullable {
		tags = append(tags, "notnull")
	}
	if col.IsUnique {
		tags = append(tags, "unique")
	}
	if col.FK != "" {
		tags = append(tags, fmt.Sprintf("fk=%s", strcase.ToCamel(col.FK)))
	}

	return strings.Join(tags, ",")
}

// FromDB generates Go struct source code for all tables in the given schema.
func FromDB(ctx context.Context, pool *pgxpool.Pool, packageName, schemaName string) (map[string]string, error) {
	rows, err := pool.Query(ctx,
		"SELECT tablename FROM pg_tables WHERE schemaname=$1", schemaName)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tableName string
	res := make(map[string]string)

	for rows.Next() {
		err = rows.Scan(&tableName)
		if err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}

		cols, err := queryColumns(ctx, pool, tableName)
		if err != nil {
			return nil, err
		}

		res[tableName] = Gen(packageName, strcase.ToCamel(tableName), cols)
	}

	return res, nil
}

// queryColumns fetches column metadata, PK, unique, and FK info for a table.
func queryColumns(ctx context.Context, pool *pgxpool.Pool, tableName string) ([]Col, error) {
	// Columns
	colRows, err := pool.Query(ctx,
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

	// Primary keys
	pkSet, err := queryConstraintColumns(ctx, pool, tableName, "PRIMARY KEY")
	if err != nil {
		return nil, err
	}

	// Unique constraints
	uniqueSet, err := queryConstraintColumns(ctx, pool, tableName, "UNIQUE")
	if err != nil {
		return nil, err
	}

	// Foreign keys
	fkMap, err := queryForeignKeys(ctx, pool, tableName)
	if err != nil {
		return nil, err
	}

	// Merge constraint info into columns
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

// queryConstraintColumns returns a set of column names for the given constraint type.
func queryConstraintColumns(ctx context.Context, pool *pgxpool.Pool, tableName, constraintType string) (map[string]bool, error) {
	rows, err := pool.Query(ctx,
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

// queryForeignKeys returns a map of column_name → referenced_table_name.
func queryForeignKeys(ctx context.Context, pool *pgxpool.Pool, tableName string) (map[string]string, error) {
	rows, err := pool.Query(ctx,
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
