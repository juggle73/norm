package norm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
)

// Model a struct to cache your struct reflect data
type Model struct {
	table          string
	valType        reflect.Type
	fields         []*Field
	fieldByAnyName map[string]*Field
	mut            sync.RWMutex

	config *Config
	pk     []string
	unique []string
}

// NewModel creates a new Model instance
func NewModel(config *Config) *Model {
	return &Model{
		config: config,
	}
}

// Parse parses an obj struct fields and save it to Model
func (m *Model) Parse(obj any, table string) error {
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
	m.fields = make([]*Field, 0)
	m.fieldByAnyName = make(map[string]*Field)

	m.valType = val.Type()

	m.pk = make([]string, 0)
	m.unique = make([]string, 0)

	m.parseFields(val.Type())

	return nil
}

// parseFields recursively parses struct fields, including embedded structs
func (m *Model) parseFields(t reflect.Type) {
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

		field := &Field{
			valType:   f.Type,
			name:      f.Name,
			dbName:    tagValues["dbName"],
			tagValues: tagValues,
		}

		m.fields = append(m.fields, field)
		m.fieldByAnyName[strcase.ToLowerCamel(field.name)] = field
		m.fieldByAnyName[field.dbName] = field
		m.fieldByAnyName[field.name] = field

		if _, hasPk := tagValues["pk"]; hasPk {
			m.pk = append(m.pk, field.dbName)
		}
		if _, hasUnique := tagValues["unique"]; hasUnique {
			m.unique = append(m.unique, field.dbName)
		}
	}
}

// filteredFields returns fields filtered by options and the composed options.
// Must be called under m.mut.RLock.
func (m *Model) filteredFields(opts ...Option) ([]*Field, ComposedOptions) {
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
func (m *Model) Fields(opts ...Option) string {
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
func (m *Model) UpdateFields(opts ...Option) (string, int) {
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
func (m *Model) Binds(opts ...Option) string {
	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)

	res := make([]string, 0, len(ff))
	for i := range ff {
		res = append(res, fmt.Sprintf("$%d", i+1))
	}

	return strings.Join(res, ", ")
}

// Pointers returns slice of field pointers for obj, excluding specified in the parameter exclude.
// Panics if obj is not a pointer to struct.
func (m *Model) Pointers(obj any, opts ...Option) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("Pointers: object must be a pointer to struct")
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, co := m.filteredFields(opts...)
	val = val.Elem()

	res := make([]any, 0, len(ff)+len(co.AddTargets))
	for _, f := range ff {
		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	for _, p := range co.AddTargets {
		res = append(res, p)
	}

	return res
}

// Pointer find field by name in obj and returns field pointer.
// Panics if obj is not a pointer to struct.
func (m *Model) Pointer(obj any, name string) any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("Pointer: object must be a pointer to struct")
	}

	return val.Elem().FieldByName(name).Addr().Interface()
}

// Values returns slice of field values as interface{} for obj, excluding specified in the parameter exclude.
// Panics if obj is not a pointer to struct.
func (m *Model) Values(obj any, opts ...Option) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("Values: object must be a pointer to struct")
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	ff, _ := m.filteredFields(opts...)
	val = val.Elem()

	res := make([]any, 0, len(ff))
	for _, f := range ff {
		res = append(res, val.FieldByName(f.name).Interface())
	}

	return res
}

// NewInstance creates and returns new instance of struct
func (m *Model) NewInstance() any {
	return reflect.New(m.valType).Interface()
}

// Table returns model database table name
func (m *Model) Table() string {
	return m.table
}

// FieldByName trying to find field by name and returns the *Field or error
func (m *Model) FieldByName(name string) (*Field, bool) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	v, ok := m.fieldByAnyName[name]
	return v, ok
}

// Returning validates field names and returns a RETURNING clause string.
// Panics if a field is not found in the model.
func (m *Model) Returning(opts ...Option) string {
	co := ComposeOptions(opts...)
	if len(co.Returning) == 0 {
		return ""
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	res := make([]string, 0, len(co.Returning))
	for _, name := range co.Returning {
		name = strings.TrimSpace(name)
		field, ok := m.fieldByAnyName[name]
		if !ok {
			panic(fmt.Sprintf("Returning: unknown field %q", name))
		}
		res = append(res, field.dbName)
	}

	return "RETURNING " + strings.Join(res, ", ")
}

// LimitOffset returns a LIMIT/OFFSET clause string from options.
func (m *Model) LimitOffset(opts ...Option) string {
	co := ComposeOptions(opts...)

	var parts []string
	if co.Limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", co.Limit))
	}
	if co.Offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", co.Offset))
	}

	return strings.Join(parts, " ")
}

// FieldDescriptions returns the slice of *Field containing all model fields
func (m *Model) FieldDescriptions() []*Field {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.fields
}

// OrderBy parses and validates an order by clause string.
// Each entry must be "fieldName [ASC|DESC]". Field names are validated
// against the model and converted to database column names.
// Panics if a field is not found or direction is invalid.
func (m *Model) OrderBy(orderBy string) string {
	orderBy = strings.TrimSpace(orderBy)
	if orderBy == "" {
		return ""
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

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
