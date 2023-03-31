package norm

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// CreateSQL returns INSERT clause for Model
//
//	exclude - exclude fields comma-separated list
//	returning - comma-separated list of returning fields
func (m *Model) CreateSQL(exclude, returning string) string {
	dbNames := m.DbNames(exclude, "")
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		m.table,
		strings.Join(dbNames, ", "),
		Binds(len(dbNames)),
	)

	if returning != "" {
		sql = fmt.Sprintf("%s RETURNING %s", sql, returning)
	}

	return sql
}

// ReadSQL generates SELECT statement with conditions and return statement and binds
//
//	where - string containing conditions, e.g. "id=?"
func (m *Model) ReadSQL(where string) (string, []any) {
	sql := fmt.Sprintf("SELECT %s FROM %s",
		strings.Join(m.DbNames("", ""), ", "),
		m.table)

	bind := 1

	for i := 0; i < len(where); i++ {
		if where[i] == '?' && (i == len(where)-1 || where[i+1] == ' ' || where[i+1] == '\n') {
			where = fmt.Sprintf("%s$%d%s", where[:i], bind, where[i+1:])
			bind++
		}
	}

	if where != "" {
		sql = fmt.Sprintf("%s WHERE %s", sql, where)
	}

	return sql, m.Pointers("")
}

// UpdateSQL generates UPDATE SQL statement
//
//	data - stringified JSON object with new values
func (m *Model) UpdateSQL(data []byte, where, returning string) (sql string, binds []any, err error) {

	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprintf("was panic, recovered value: %v", r))
		}
	}()

	sets := make([]string, 0)
	binds = make([]any, 0)
	bind := 1

	dataObj := make(map[string]any)

	err = json.Unmarshal(data, &dataObj)
	if err != nil {
		return
	}

	for k, v := range dataObj {
		f := m.fieldByAnyName[k]
		if f == nil || f.hasTag("noupdate") {
			continue
		}
		val := reflect.ValueOf(m.currentObject).Elem()
		rField := val.FieldByName(f.name)
		if rField.CanSet() {
			rField.Set(reflect.ValueOf(v).Convert(rField.Type()))
			sets = append(sets, fmt.Sprintf("%s=$%d", f.dbName, bind))
			binds = append(binds, rField.Interface())
			bind++
		}
	}

	if len(sets) == 0 {
		err = errors.New("no data to update")
	}

	for i := range where {
		if where[i] == '?' && (i == len(where)-1 || where[i+1] == ' ') {
			where = fmt.Sprintf("%s$%d%s", where[:i], bind, where[i+1:])
			bind++
		}
	}

	sql = fmt.Sprintf("UPDATE %s SET %s",
		m.table,
		strings.Join(sets, ", "))

	if where != "" {
		sql = fmt.Sprintf("%s WHERE %s", sql, where)
	}
	if returning != "" {
		sql = fmt.Sprintf("%s RETURNING %s", sql, returning)
	}

	return
}
