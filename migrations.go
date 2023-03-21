package norm

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"reflect"
	"strings"
	"time"
)

// dbTypes is map of PostgreSQL types for reflect.Kind
var dbTypes = map[reflect.Kind]string{
	reflect.Int64: "bigint",
	reflect.Int:   "integer",
	reflect.Bool:  "boolean",
}

// CreateSQL generates CREATE TABLE sql expression for Model
func (m *Model) CreateSQL(table string) string {
	m.pk = make([]string, 0)
	m.unique = make([]string, 0)

	fields := make([]string, 0)

	for _, f := range m.fields {
		fields = append(fields, m.fieldCreate(f.valType, f))
	}

	if len(m.pk) > 0 {
		fields = append(fields,
			fmt.Sprintf("CONSTRAINT %s_pkey PRIMARY KEY(%s)", table, strings.Join(m.pk, ", ")))
	}
	for _, uField := range m.unique {
		fields = append(fields,
			fmt.Sprintf("CONSTRAINT unique_%s_%s UNIQUE(%s)", table, uField, uField))
	}

	sql := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s
)`, table, strings.Join(fields, ",\n\t"))

	return sql
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
				panic("fieldCreate: invalid data type: " + t.Kind().String())
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
