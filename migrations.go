package norm

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"reflect"
	"strings"
	"time"
)

// dbTypes is map of some reflect.Kind to PostgreSQL types
var dbTypes = map[reflect.Kind]string{
	reflect.Int64: "bigint",
	reflect.Int:   "integer",
	reflect.Bool:  "boolean",
}

// CreateTableSQL generates CREATE TABLE sql expression for Model
func (m *Model) CreateTableSQL() string {
	m.pk = make([]string, 0)
	m.unique = make([]string, 0)

	fields := make([]string, 0)

	for _, f := range m.fields {
		fields = append(fields, m.fieldCreate(f.valType, f))
	}

	if len(m.pk) > 0 {
		fields = append(fields,
			fmt.Sprintf("CONSTRAINT %s_pkey PRIMARY KEY(%s)", m.table, strings.Join(m.pk, ", ")))
	}
	for _, uField := range m.unique {
		fields = append(fields,
			fmt.Sprintf("CONSTRAINT unique_%s_%s UNIQUE(%s)", m.table, uField, uField))
	}

	sql := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s
)`, m.table, strings.Join(fields, ",\n\t"))

	return sql
}

// Migrate compares db field names with model fields and generates SQL statements to create missing table fields
//
//	Important! Migrate does not support field deletion and field data type change
func (m *Model) Migrate(dbFieldNames []string) []string {
	m.pk = make([]string, 0)
	m.unique = make([]string, 0)

	statements := make([]string, 0)

	// New table
	if dbFieldNames == nil || len(dbFieldNames) == 0 {
		statements = append(statements, m.CreateTableSQL())
		return statements
	}

	for _, f := range m.fields {
		if !has(dbFieldNames, f.dbName) {
			sql := fmt.Sprintf("ALTER TABLE %s ADD %s", m.table, m.fieldCreate(f.valType, f))
			statements = append(statements, sql)
		}
	}

	if len(m.pk) > 0 {
		sql := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", m.table, strings.Join(m.pk, ", "))
		statements = append(statements, sql)
	}
	for _, uField := range m.unique {
		sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT unique_%s_%s UNIQUE (%s)",
			m.table, m.table, uField, uField)
		statements = append(statements, sql)
	}

	return statements
}

// fieldCreate creates a sql for field
func (m *Model) fieldCreate(t reflect.Type, f *Field) string {
	res := ""

	var (
		dt string
		ok bool
	)

	dt, ok = f.tagValues["dbType"]

	if !ok {
		switch t.Kind() {
		case reflect.Pointer:
			return m.fieldCreate(t.Elem(), f)
		case reflect.String:
			dt = m.config.DefaultString
		case reflect.Struct:
			if t == reflect.TypeOf(time.Time{}) || t == reflect.TypeOf(pgtype.Timestamptz{}) {
				dt = "timestamp with time zone"
			}
		default:
			dt, ok = dbTypes[t.Kind()]
			if !ok {
				dt = fmt.Sprintf("<not supported type %s>", t.Kind().String())
			}
		}
	}

	if _, ok = f.tagValues["pk"]; ok {
		m.pk = append(m.pk, f.dbName)
	}
	if _, ok = f.tagValues["unique"]; ok {
		m.unique = append(m.unique, f.dbName)
	}

	res = fmt.Sprintf("%s %s", f.dbName, dt)

	if _, ok = f.tagValues["notnull"]; ok {
		res += " NOT NULL"
	}

	var v string
	if v, ok = f.tagValues["default"]; ok {
		res = fmt.Sprintf("%s DEFAULT %s", res, v)
	}

	return res
}
