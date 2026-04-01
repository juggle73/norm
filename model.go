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

	c := val.NumField()
	for i := 0; i < c; i++ {
		f := val.Type().Field(i)

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
	}

	return nil
}

// Fields returns slice of fields database names with prefix in snake-case, excluding
// specified in the parameter exclude
func (m *Model) Fields(opts ...Option) string {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	res := make([]string, 0)
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}

		res = append(res, co.Prefix+f.dbName)
	}

	return strings.Join(res, ", ")
}

// UpdateFields returns comma separated fields database names in snake-case
// in "<field db name>=$<bind num>" format, excluding specified in the parameter exclude,
// and next bind number
func (m *Model) UpdateFields(opts ...Option) (string, int) {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	bind := 1
	res := make([]string, 0)
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}

		res = append(res, fmt.Sprintf("%s=$%d", f.dbName, bind))
		bind++
	}

	return strings.Join(res, ", "), bind
}

// Binds returns comma separated binds in format $<bind no.> for count of fields, excluding
// specified in the parameter exclude
func (m *Model) Binds(opts ...Option) string {
	co := ComposeOptions(opts...)

	m.mut.RLock()
	defer m.mut.RUnlock()

	res := make([]string, 0)
	idx := 1
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}

		res = append(res, fmt.Sprintf("$%d", idx))
		idx++
	}

	return strings.Join(res, ", ")
}

// Pointers returns slice of field pointers for obj, excluding specified in the parameter exclude
func (m *Model) Pointers(obj any, opts ...Option) ([]any, error) {
	co := ComposeOptions(opts...)

	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		return nil, errors.New("Pointers: object must be a pointer to struct")
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	val = val.Elem()

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	for _, p := range co.AddTargets {
		res = append(res, p)
	}

	return res, nil
}

// Pointer find field by name in obj and returns field pointer
func (m *Model) Pointer(obj any, name string) (any, error) {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		return nil, errors.New("Pointer: object must be a pointer to struct")
	}

	return val.Elem().FieldByName(name).Addr().Interface(), nil
}

// Values returns slice of field values as interface{} for obj, excluding specified in the parameter exclude
func (m *Model) Values(obj any, opts ...Option) ([]any, error) {
	co := ComposeOptions(opts...)

	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		return nil, errors.New("Values: object must be a pointer to struct")
	}

	m.mut.RLock()
	defer m.mut.RUnlock()

	val = val.Elem()

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(co.Exclude, f.dbName) {
			continue
		}
		if len(co.Fields) > 0 && !has(co.Fields, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Interface())
	}

	return res, nil
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

// FieldDescriptions returns the slice of *Field containing all model fields
func (m *Model) FieldDescriptions() []*Field {
	m.mut.RLock()
	defer m.mut.RUnlock()
	return m.fields
}
