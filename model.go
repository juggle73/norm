package norm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
)

// modelMeta caches struct reflection metadata. Thread-safe for concurrent reads.
type modelMeta struct {
	table          string
	valType        reflect.Type
	fields         []*Field
	fieldByAnyName map[string]*Field
	mut            sync.RWMutex

	config *Config
	pk     []string
}

// Model binds cached metadata to a specific struct instance.
// Not safe for concurrent use.
type Model struct {
	*modelMeta
	val reflect.Value
}

// newModelMeta creates a new modelMeta instance
func newModelMeta(config *Config) *modelMeta {
	return &modelMeta{
		config: config,
	}
}

// Parse parses an obj struct fields and save it to modelMeta
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

// parseFields recursively parses struct fields, including embedded structs
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

// filteredFields returns fields filtered by options and the composed options.
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

// Fields returns slice of fields database names with prefix in snake-case, excluding
// specified in the parameter exclude
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

// UpdateFields returns comma separated fields database names in snake-case
// in "<field db name>=$<bind num>" format, excluding specified in the parameter exclude,
// and next bind number
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

// Binds returns comma separated binds in format $<bind no.> for count of fields, excluding
// specified in the parameter exclude
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

// Pointers returns slice of field pointers for the bound struct instance.
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

// Pointer returns a pointer to the named field of the bound struct instance.
func (m *Model) Pointer(name string) any {
	return m.val.FieldByName(name).Addr().Interface()
}

// Values returns slice of field values for the bound struct instance.
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

// Select builds a SELECT SQL string from options.
// Supports: Exclude, Fields, Prefix, Where, Order, Limit, Offset.
func (m *Model) Select(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	// SELECT columns
	cols := make([]string, 0, len(ff))
	for _, f := range ff {
		cols = append(cols, co.Prefix+f.dbName)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(cols, ", "), m.table)

	var args []any

	// WHERE
	if co.Where != nil {
		whereStr, _ := co.Where.Build(1)
		sql += " WHERE " + whereStr
		args = append(args, co.Where.Args...)
	}

	// ORDER BY
	if co.OrderBy != "" {
		sql += " ORDER BY " + m.orderBySQL(co.OrderBy)
	}

	// LIMIT / OFFSET
	if co.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", co.Limit)
	}
	if co.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", co.Offset)
	}

	return sql, args, nil
}

// Insert builds an INSERT SQL string and returns values from the bound struct.
// Supports: Exclude, Fields, Returning.
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

// Update builds an UPDATE SQL string and returns combined args (SET values + WHERE args).
// Supports: Exclude, Fields, Where, Returning.
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

	// WHERE
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

// Delete builds a DELETE SQL string and returns WHERE args.
// Supports: Where, Returning.
func (m *Model) Delete(opts ...Option) (string, []any, error) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	sql := fmt.Sprintf("DELETE FROM %s", m.table)

	var args []any

	// WHERE
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

// returningSQL builds a RETURNING clause from composed options. Must be called under m.mut.RLock.
// Returns the clause string (including " RETURNING " prefix) and error.
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

// orderBySQL validates and renders ORDER BY clause. Must be called under m.mut.RLock.
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

// NewInstance creates and returns new instance of struct
func (m *modelMeta) NewInstance() any {
	return reflect.New(m.valType).Interface()
}

// Table returns model database table name
func (m *modelMeta) Table() string {
	return m.table
}

// FieldByName trying to find field by name and returns the *Field or error
func (m *modelMeta) FieldByName(name string) (*Field, bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	v, ok := m.fieldByAnyName[name]
	return v, ok
}

// Returning validates field names and returns a RETURNING clause string.
// Fields is a comma-separated list of field names (struct name, camelCase, or db name).
// Returns empty string if fields is empty.
// Panics if a field is not found in the model.
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

// LimitOffset returns a LIMIT/OFFSET clause string.
// Pass 0 to omit either clause.
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

// FieldDescriptions returns the slice of *Field containing all model fields
func (m *modelMeta) FieldDescriptions() []*Field {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.fields
}

// OrderBy parses and validates an order by clause string.
// Each entry must be "fieldName [ASC|DESC]". Field names are validated
// against the model and converted to database column names.
// Panics if a field is not found or direction is invalid.
func (m *modelMeta) OrderBy(orderBy string) string {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" {
		return ""
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	return m.orderBySQL(orderBy)
}
