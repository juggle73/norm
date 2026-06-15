// Package gen generates Go struct source code from database schemas.
//
// The package-level [FromDB] and [Gen] use PostgreSQL. For other dialects
// create a [Generator] with [NewGenerator] (PostgreSQL, SQLite and MySQL
// supported).
//
// Usage:
//
//	results, err := gen.FromDB(ctx, pool, "models", "public")     // PostgreSQL
//	results, err := gen.NewGenerator(norm.SQLite).FromDB(ctx, db, "models", "")
//	results, err := gen.NewGenerator(norm.MySQL).FromDB(ctx, db, "models", "")
//	for tableName, source := range results {
//	    os.WriteFile(tableName+".go", []byte(source), 0644)
//	}
package gen

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/juggle73/norm/v4"
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
	"smallint":  {"int16", "", false},
	"integer":   {"int", "", false},
	"bigint":    {"int64", "", false},
	"serial":    {"int", "", false},
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

// Generator generates Go struct source code for a specific SQL dialect.
// Create one with [NewGenerator]; the zero value is not usable.
type Generator struct {
	schema schema
}

// NewGenerator returns a [Generator] for the given dialect (use
// [github.com/juggle73/norm/v4.PostgreSQL], [github.com/juggle73/norm/v4.SQLite]).
//
//	g := gen.NewGenerator(norm.SQLite)
//	results, err := g.FromDB(ctx, db, "models", "")
func NewGenerator(d norm.Dialect) *Generator {
	return &Generator{schema: schemaFor(d)}
}

// Gen generates Go struct source code from column definitions, mapping
// database types according to the generator's dialect.
func (g *Generator) Gen(packageName, structName string, cols []Col) string {
	return genStruct(g.schema, packageName, structName, cols)
}

// Gen generates Go struct source code from column definitions using
// PostgreSQL type mapping. For other dialects use [NewGenerator].
func Gen(packageName, structName string, cols []Col) string {
	return genStruct(postgresGen{}, packageName, structName, cols)
}

// genStruct renders a Go struct for the given schema dialect.
func genStruct(sch schema, packageName, structName string, cols []Col) string {
	imports := make(map[string]bool)
	structStr := fmt.Sprintf("type %s struct {\n", structName)

	for _, col := range cols {
		info, ok := sch.goType(col.DataType)
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

// FromDB generates Go struct source code for all tables in the given schema
// using PostgreSQL introspection. It accepts *sql.DB — any PostgreSQL driver
// works (pgx/stdlib, lib/pq, etc.). For other dialects use [NewGenerator].
func FromDB(ctx context.Context, db *sql.DB, packageName, schemaName string) (map[string]string, error) {
	return fromDB(ctx, db, postgresGen{}, packageName, schemaName)
}

// FromDB generates Go struct source code for all tables in the given schema,
// introspecting the database according to the generator's dialect.
//
// For SQLite the schemaName argument is ignored.
func (g *Generator) FromDB(ctx context.Context, db *sql.DB, packageName, schemaName string) (map[string]string, error) {
	return fromDB(ctx, db, g.schema, packageName, schemaName)
}

func fromDB(ctx context.Context, db *sql.DB, sch schema, packageName, schemaName string) (map[string]string, error) {
	tables, err := sch.listTables(ctx, db, schemaName)
	if err != nil {
		return nil, err
	}

	res := make(map[string]string)
	for _, tableName := range tables {
		cols, err := sch.queryColumns(ctx, db, tableName)
		if err != nil {
			return nil, err
		}
		res[tableName] = genStruct(sch, packageName, strcase.ToCamel(tableName), cols)
	}

	return res, nil
}
