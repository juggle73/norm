package norm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
)

// modelMeta caches struct reflection metadata (field names, types, tags).
// It is created once per struct type and shared across all [Model] instances.
// Thread-safe for concurrent reads.
type modelMeta struct {
	table          string
	valType        reflect.Type
	fields         []*Field
	fieldByAnyName map[string]*Field
	mut            sync.RWMutex

	config *Config
	pk     []string
}

// Model binds cached metadata to a specific struct instance. It provides
// methods for building SQL queries and extracting values/pointers from
// the bound struct.
//
// Model is not safe for concurrent use. Each goroutine should obtain its
// own Model via [Norm.M].
type Model struct {
	*modelMeta
	val reflect.Value
}

// newModelMeta creates a new modelMeta instance with the given config.
func newModelMeta(config *Config) *modelMeta {
	return &modelMeta{
		config: config,
	}
}

// Parse extracts field metadata from obj and stores it in the modelMeta.
// obj must be a struct or pointer to struct. If table is empty, the table
// name is derived from the struct name in snake_case.
func (m *modelMeta) Parse(obj any, table string) error {
	val := reflect.Indirect(reflect.ValueOf(obj))
	if val.Kind() != reflect.Struct {
		return errors.New("object must be a struct or pointer to struct")
	}

	m.mut.Lock()
	defer m.mut.Unlock()

	if table == "" {
		m.table = strcase.ToSnake(val.Type().Name())
	} else {
		m.table = table
	}
	if !isValidIdentifier(m.table) {
		panic(fmt.Sprintf("invalid table name %q: must contain only [a-zA-Z0-9_]", m.table))
	}
	m.fields = make([]*Field, 0)
	m.fieldByAnyName = make(map[string]*Field)

	m.valType = val.Type()

	m.pk = make([]string, 0)

	m.parseFields(val.Type())

	return nil
}

// parseFields recursively parses struct fields, including embedded structs.
func (m *modelMeta) parseFields(t reflect.Type) {
	c := t.NumField()
	for i := 0; i < c; i++ {
		f := t.Field(i)

		// Recurse into embedded (anonymous) structs
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if f.Anonymous && ft.Kind() == reflect.Struct {
			m.parseFields(ft)
			continue
		}

		tagValues, ok := parseNormTag(f)
		if !ok {
			continue
		}

		dbName := tagValues["dbName"]
		if !isValidIdentifier(dbName) {
			panic(fmt.Sprintf("invalid db column name %q for field %s: must contain only [a-zA-Z0-9_]", dbName, f.Name))
		}

		field := &Field{
			valType:   f.Type,
			name:      f.Name,
			dbName:    dbName,
			tagValues: tagValues,
		}

		m.fields = append(m.fields, field)
		m.fieldByAnyName[strcase.ToLowerCamel(field.name)] = field
		m.fieldByAnyName[field.dbName] = field
		m.fieldByAnyName[field.name] = field

		if _, hasPk := tagValues["pk"]; hasPk {
			m.pk = append(m.pk, field.dbName)
		}
	}
}

// filteredFields returns fields filtered by Exclude/Fields options.
// Must be called under m.mut.RLock.
func (m *modelMeta) filteredFields(opts ...Option) ([]*Field, ComposedOptions) {
	co := ComposeOptions(opts...)

	res := make([]*Field, 0, len(m.fields))
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}
		res = append(res, f)
	}

	return res, co
}

// Fields returns a comma-separated list of column names in snake_case.
// Supports [Exclude], [Fields], and [Prefix] options.
//
//	m.Fields()                          // "id, name, email"
//	m.Fields(norm.Exclude("id"))        // "name, email"
//	m.Fields(norm.Prefix("u."))         // "u.id, u.name, u.email"
func (m *modelMeta) Fields(opts ...Option) string {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, co := m.filteredFields(opts...)

	res := make([]string, 0, len(ff))
	for _, f := range ff {
		res = append(res, co.Prefix+f.dbName)
	}

	return strings.Join(res, ", ")
}

// UpdateFields returns a SET clause string ("name=$1, email=$2") and the
// next bind parameter number. Supports [Exclude] and [Fields] options.
//
//	set, nextBind := m.UpdateFields(norm.Exclude("id"))
//	// set = "name=$1, email=$2", nextBind = 3
func (m *modelMeta) UpdateFields(opts ...Option) (string, int) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	res := make([]string, 0, len(ff))
	for i, f := range ff {
		res = append(res, fmt.Sprintf("%s=$%d", f.dbName, i+1))
	}

	return strings.Join(res, ", "), len(ff) + 1
}

// Binds returns a comma-separated list of bind placeholders ($1, $2, ...).
// Supports [Exclude] and [Fields] options.
//
//	m.Binds()                    // "$1, $2, $3"
//	m.Binds(norm.Exclude("id")) // "$1, $2"
func (m *modelMeta) Binds(opts ...Option) string {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	res := make([]string, 0, len(ff))
	for i := range ff {
		res = append(res, fmt.Sprintf("$%d", i+1))
	}

	return strings.Join(res, ", ")
}

// Pointers returns a slice of pointers to the bound struct's fields,
// suitable for passing to rows.Scan(). Struct fields (except time.Time)
// are wrapped in a JSON scanner automatically.
// Supports [Exclude], [Fields], and [AddTargets] options.
//
//	err := row.Scan(m.Pointers()...)
//	err := row.Scan(m.Pointers(norm.AddTargets(&totalCount))...)
func (m *Model) Pointers(opts ...Option) []any {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, co := m.filteredFields(opts...)

	res := make([]any, 0, len(ff)+len(co.AddTargets))
	for _, f := range ff {
		ptr := m.val.FieldByName(f.name).Addr().Interface()
		if f.isJSON() {
			res = append(res, &jsonScanner{target: ptr})
		} else {
			res = append(res, ptr)
		}
	}

	for _, p := range co.AddTargets {
		res = append(res, p)
	}

	return res
}

// Pointer returns a pointer to a single named field of the bound struct.
// Panics if the field name is not found — this is a programmer error.
//
//	err := row.Scan(m.Pointer("Id"))
func (m *Model) Pointer(name string) any {
	return m.val.FieldByName(name).Addr().Interface()
}

// Values returns a slice of field values from the bound struct instance.
// Struct fields (except time.Time) are marshaled to JSON bytes automatically.
// Supports [Exclude] and [Fields] options.
//
//	_, err := pool.Exec(ctx, sql, m.Values(norm.Exclude("id"))...)
func (m *Model) Values(opts ...Option) []any {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	res := make([]any, 0, len(ff))
	for _, f := range ff {
		val := m.val.FieldByName(f.name).Interface()
		if f.isJSON() {
			b, err := jsonValue(val)
			if err != nil {
				panic(fmt.Sprintf("Values: json marshal field %q: %v", f.name, err))
			}
			res = append(res, b)
		} else {
			res = append(res, val)
		}
	}

	return res
}

// Select builds a full SELECT query from the bound model.
// Returns the SQL string, positional arguments, and any error.
// Supports [Exclude], [Fields], [Prefix], [Where], [Order], [Limit], [Offset] options.
//
//	sql, args, _ := m.Select(
//	    norm.Where("active = ?", true),
//	    norm.Order("Name DESC"),
//	    norm.Limit(10),
//	)
//	// "SELECT id, name, email FROM users WHERE active=$1 ORDER BY name DESC LIMIT 10"
func (m *Model) Select(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	cols := make([]string, 0, len(ff))
	for _, f := range ff {
		cols = append(cols, co.Prefix+f.dbName)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), m.table)

	var args []any

	if co.Where != nil {
		whereStr, _ := co.Where.Build(1)
		sql += " WHERE " + whereStr
		args = append(args, co.Where.Args...)
	}

	if co.OrderBy != "" {
		sql += " ORDER BY " + m.orderBySQL(co.OrderBy)
	}

	if co.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", co.Limit)
	}
	if co.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", co.Offset)
	}

	return sql, args, nil
}

// Insert builds a full INSERT query and returns the SQL string and values
// from the bound struct. Struct fields are automatically JSON-marshaled.
// Supports [Exclude], [Fields], and [Returning] options.
//
//	sql, vals, _ := m.Insert(norm.Exclude("id"), norm.Returning("Id"))
//	// "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id"
func (m *Model) Insert(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	cols := make([]string, 0, len(ff))
	binds := make([]string, 0, len(ff))
	vals := make([]any, 0, len(ff))

	for i, f := range ff {
		cols = append(cols, f.dbName)
		binds = append(binds, fmt.Sprintf("$%d", i+1))
		val := m.val.FieldByName(f.name).Interface()
		if f.isJSON() {
			b, err := jsonValue(val)
			if err != nil {
				return "", nil, fmt.Errorf("Insert: json marshal field %q: %w", f.name, err)
			}
			vals = append(vals, b)
		} else {
			vals = append(vals, val)
		}
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		m.table,
		strings.Join(cols, ", "),
		strings.Join(binds, ", "),
	)

	retSQL, err := m.returningSQL(co.Returning)
	if err != nil {
		return "", nil, err
	}
	sql += retSQL

	return sql, vals, nil
}

// Update builds a full UPDATE query and returns the SQL string and combined
// args (SET values followed by WHERE args). Struct fields are automatically
// JSON-marshaled. Bind numbering is chained: SET uses $1..$N, WHERE
// continues from $N+1.
// Supports [Exclude], [Fields], [Where], and [Returning] options.
//
//	sql, args, _ := m.Update(norm.Exclude("id"), norm.Where("id = ?", user.Id))
//	// "UPDATE users SET name=$1, email=$2 WHERE id=$3"
func (m *Model) Update(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	if len(ff) == 0 {
		return "", nil, errors.New("Update: no fields to set")
	}

	setCols := make([]string, 0, len(ff))
	vals := make([]any, 0, len(ff))

	for i, f := range ff {
		setCols = append(setCols, fmt.Sprintf("%s=$%d", f.dbName, i+1))
		val := m.val.FieldByName(f.name).Interface()
		if f.isJSON() {
			b, err := jsonValue(val)
			if err != nil {
				return "", nil, fmt.Errorf("Update: json marshal field %q: %w", f.name, err)
			}
			vals = append(vals, b)
		} else {
			vals = append(vals, val)
		}
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", m.table, strings.Join(setCols, ", "))

	if co.Where != nil {
		whereStr, _ := co.Where.Build(len(ff) + 1)
		sql += " WHERE " + whereStr
		vals = append(vals, co.Where.Args...)
	}

	retSQL, err := m.returningSQL(co.Returning)
	if err != nil {
		return "", nil, err
	}
	sql += retSQL

	return sql, vals, nil
}

// Delete builds a full DELETE query and returns the SQL string and WHERE args.
// Supports [Where] and [Returning] options.
//
//	sql, args, _ := m.Delete(norm.Where("id = ?", 42))
//	// "DELETE FROM users WHERE id=$1"
func (m *Model) Delete(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	sql := fmt.Sprintf("DELETE FROM %s", m.table)

	var args []any

	if co.Where != nil {
		whereStr, _ := co.Where.Build(1)
		sql += " WHERE " + whereStr
		args = append(args, co.Where.Args...)
	}

	retSQL, err := m.returningSQL(co.Returning)
	if err != nil {
		return "", nil, err
	}
	sql += retSQL

	return sql, args, nil
}

// returningSQL builds a RETURNING clause from field names.
// Must be called under m.mut.RLock.
func (m *modelMeta) returningSQL(returning []string) (string, error) {
	if len(returning) == 0 {
		return "", nil
	}
	ret := make([]string, 0, len(returning))
	for _, name := range returning {
		name = strings.TrimSpace(name)
		field, ok := m.fieldByAnyName[name]
		if !ok {
			return "", fmt.Errorf("Returning: unknown field %q", name)
		}
		ret = append(ret, field.dbName)
	}
	return " RETURNING " + strings.Join(ret, ", "), nil
}

// orderBySQL validates and renders an ORDER BY clause.
// Must be called under m.mut.RLock.
func (m *modelMeta) orderBySQL(orderBy string) string {
	parts := strings.Split(orderBy, ",")
	res := make([]string, 0, len(parts))

	for _, part := range parts {
		tokens := strings.Fields(strings.TrimSpace(part))
		if len(tokens) == 0 {
			continue
		}

		fieldName := tokens[0]
		direction := "ASC"

		if len(tokens) == 2 {
			direction = strings.ToUpper(tokens[1])
		} else if len(tokens) > 2 {
			panic(fmt.Sprintf("OrderBy: invalid format %q", part))
		}

		if direction != "ASC" && direction != "DESC" {
			panic(fmt.Sprintf("OrderBy: invalid direction %q, must be ASC or DESC", direction))
		}

		field, ok := m.fieldByAnyName[fieldName]
		if !ok {
			panic(fmt.Sprintf("OrderBy: unknown field %q", fieldName))
		}

		res = append(res, field.dbName+" "+direction)
	}

	return strings.Join(res, ", ")
}

// NewInstance creates and returns a new zero-value pointer to the struct type
// that this model represents.
//
//	inst := m.NewInstance() // returns *User (zero value)
func (m *modelMeta) NewInstance() any {
	return reflect.New(m.valType).Interface()
}

// Table returns the database table name for this model.
func (m *modelMeta) Table() string {
	return m.table
}

// FieldByName looks up a field by any name format (struct name, camelCase,
// or snake_case db name). Returns the [Field] and true if found.
func (m *modelMeta) FieldByName(name string) (*Field, bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	v, ok := m.fieldByAnyName[name]
	return v, ok
}

// Returning validates field names and returns a RETURNING clause string
// (e.g. "RETURNING id, name"). Fields is a comma-separated list of field
// names in any format (struct name, camelCase, or db name).
// Returns empty string if fields is empty.
// Panics if a field is not found — this is a programmer error.
//
//	m.Returning("Id")          // "RETURNING id"
//	m.Returning("Id, Email")   // "RETURNING id, email"
func (m *modelMeta) Returning(fields string) string {
	fields = strings.TrimSpace(fields)
	if fields == "" {
		return ""
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	parts := strings.Split(fields, ",")
	res := make([]string, 0, len(parts))
	for _, name := range parts {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		field, ok := m.fieldByAnyName[name]
		if !ok {
			panic(fmt.Sprintf("Returning: unknown field %q", name))
		}
		res = append(res, field.dbName)
	}

	if len(res) == 0 {
		return ""
	}

	return "RETURNING " + strings.Join(res, ", ")
}

// LimitOffset returns a LIMIT/OFFSET clause string. Pass 0 to omit either part.
//
//	m.LimitOffset(10, 0)   // "LIMIT 10"
//	m.LimitOffset(10, 20)  // "LIMIT 10 OFFSET 20"
func (m *modelMeta) LimitOffset(limit, offset int) string {
	var parts []string
	if limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", limit))
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", offset))
	}

	return strings.Join(parts, " ")
}

// FieldDescriptions returns the slice of all [Field] descriptors for this model.
func (m *modelMeta) FieldDescriptions() []*Field {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.fields
}

// OrderBy validates and renders an ORDER BY clause string. Field names are
// validated against the model and converted to database column names.
// Accepts any name format (struct name, camelCase, snake_case).
// Direction defaults to ASC if omitted.
// Panics if a field is not found or direction is invalid — these are
// programmer errors.
//
//	m.OrderBy("Name DESC")          // "name DESC"
//	m.OrderBy("Name ASC, Email")    // "name ASC, email ASC"
func (m *modelMeta) OrderBy(orderBy string) string {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" {
		return ""
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	return m.orderBySQL(orderBy)
}
