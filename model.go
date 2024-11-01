package norm

import (
	"errors"
	"fmt"
	"github.com/iancoleman/strcase"
	"reflect"
	"strings"
)

// Model a struct to cache your struct reflect data
type Model struct {
	table          string
	valType        reflect.Type
	fields         []*Field
	fieldByAnyName map[string]*Field

	config        *Config
	currentObject any
	pk            []string
	unique        []string
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

// DbNames returns slice of fields database names with prefix in snake-case, excluding
// specified in the parameter exclude
func (m *Model) DbNames(exclude, prefix string) []string {
	excludeArr := strings.Split(exclude, ",")

	res := make([]string, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, prefix+f.dbName)
	}

	return res
}

// DbNamesCsv returns comma separated names of fields database names with prefix in snake-case, excluding
// specified in the parameter exclude
func (m *Model) DbNamesCsv(exclude, prefix string) string {
	return strings.Join(m.DbNames(exclude, prefix), ", ")
}

// DbNamesFields returns slice of fields database names with prefix in snake-case, including
// only specified in the parameter fields
func (m *Model) DbNamesFields(fields, prefix string) []string {
	fieldsArr := strings.Split(fields, ",")

	res := make([]string, 0)
	for _, f := range m.fields {
		if has(fieldsArr, f.dbName) {
			res = append(res, prefix+f.dbName)
		}
	}

	return res
}

// DbNamesFieldsCsv returns comma separated names of fields database names with prefix in snake-case, including
// only specified in the parameter fields
func (m *Model) DbNamesFieldsCsv(fields, prefix string) string {
	return strings.Join(m.DbNamesFields(fields, prefix), ", ")
}

// DbNamesWithBinds returns slice of fields database names in snake-case
// in "<field db name>=$<bind num>" format, excluding specified in the parameter exclude
func (m *Model) DbNamesWithBinds(exclude string) []string {
	excludeArr := strings.Split(exclude, ",")

	bind := 1
	res := make([]string, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, fmt.Sprintf("%s=$%d", f.dbName, bind))
		bind++
	}

	return res
}

// DbNamesWithBindsCsv returns comma separated names of fields database names in snake-case
// in "<field db name>=$<bind num>" format, excluding specified in the parameter exclude
func (m *Model) DbNamesWithBindsCsv(exclude string) string {
	return strings.Join(m.DbNamesWithBinds(exclude), ", ")
}

// BindsCsv returns comma separated binds in format $<bind no.> for count of fields, excluding
// specified in the parameter exclude
func (m *Model) BindsCsv(exclude string) string {
	dbNames := m.DbNames(exclude, "")
	res := make([]string, 0)
	for i := range dbNames {
		res = append(res, fmt.Sprintf("$%d", i+1))
	}

	return strings.Join(res, ", ")
}

// BindsFieldsCsv returns comma separated binds in format $<bind no.> for count of fields, including
// only specified in the parameter fields
func (m *Model) BindsFieldsCsv(fields string) string {
	dbNames := m.DbNamesFields(fields, "")
	res := make([]string, 0)
	for i := range dbNames {
		res = append(res, fmt.Sprintf("$%d", i+1))
	}

	return strings.Join(res, ", ")
}

// DbNamesFieldsWithBinds returns slice of fields database names in snake-case
// in "<field db name>=$<bind num>" format, including only specified in the parameter fields
func (m *Model) DbNamesFieldsWithBinds(fields string) []string {
	fieldsArr := strings.Split(fields, ",")

	bind := 1
	res := make([]string, 0)
	for _, f := range m.fields {
		if has(fieldsArr, f.dbName) {
			res = append(res, fmt.Sprintf("%s=$%d", f.dbName, bind))
			bind++
		}
	}

	return res
}

// DbNamesFieldsWithBindsCsv returns slice of fields database names in snake-case
// in "<field db name>=$<bind num>" format, including only specified in the parameter fields
func (m *Model) DbNamesFieldsWithBindsCsv(fields string) string {
	return strings.Join(m.DbNamesFieldsWithBinds(fields), ", ")
}

// Pointers returns slice of field pointers for obj, excluding specified in the parameter exclude
//
// Deprecated: Use PointersObj instead
func (m *Model) Pointers(exclude string, add ...any) []any {
	val := reflect.ValueOf(m.currentObject)
	if !isPointerToStruct(val) {
		panic("FieldPointers: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	for _, p := range add {
		res = append(res, p)
	}

	return res
}

// PointersObj returns slice of field pointers for obj, excluding specified in the parameter exclude
func (m *Model) PointersObj(obj any, exclude string, add ...any) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("FieldPointers: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	for _, p := range add {
		res = append(res, p)
	}

	return res
}

// PointerFieldsObj returns slice of field pointers for obj, including
// only specified in the parameter fields
func (m *Model) PointerFieldsObj(obj any, include string) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("FieldPointers: object must be a pointer to struct")
	}

	val = val.Elem()

	includeArr := strings.Split(include, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if !has(includeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	return res
}

// PointerFields returns slice of field pointers for obj, including
// only specified in the parameter fields
//
// Deprecated: Use PointerFieldsObj instead
func (m *Model) PointerFields(include string) []any {
	val := reflect.ValueOf(m.currentObject)
	if !isPointerToStruct(val) {
		panic("FieldPointers: object must be a pointer to struct")
	}

	val = val.Elem()

	includeArr := strings.Split(include, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if !has(includeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	return res
}

// ValuesObj returns slice of field values as interface{} for obj, excluding specified in the parameter exclude
func (m *Model) ValuesObj(obj any, exclude string) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("FieldValues: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Interface())
	}

	return res
}

// Values returns slice of field values as interface{} for obj, excluding specified in the parameter exclude
//
// Deprecated: Use ValuesObj instead
func (m *Model) Values(exclude string) []any {
	val := reflect.ValueOf(m.currentObject)
	if !isPointerToStruct(val) {
		panic("FieldValues: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) {
			continue
		}

		res = append(res, val.FieldByName(f.name).Interface())
	}

	return res
}

// ValuesFieldsObj returns slice of field values as interface{} for obj,
// including only specified in the parameter fields
func (m *Model) ValuesFieldsObj(obj any, fields string) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("FieldValues: object must be a pointer to struct")
	}

	val = val.Elem()

	fieldsArr := strings.Split(fields, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(fieldsArr, f.dbName) {
			res = append(res, val.FieldByName(f.name).Interface())
		}
	}

	return res
}

// ValuesFields returns slice of field values as interface{} for obj,
// including only specified in the parameter fields
//
// Deprecated: Use ValuesFieldsObj instead
func (m *Model) ValuesFields(fields string) []any {
	val := reflect.ValueOf(m.currentObject)
	if !isPointerToStruct(val) {
		panic("FieldValues: object must be a pointer to struct")
	}

	val = val.Elem()

	fieldsArr := strings.Split(fields, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(fieldsArr, f.dbName) {
			res = append(res, val.FieldByName(f.name).Interface())
		}
	}

	return res
}

// NewInstance creates and returns new instance of struct
func (m *Model) NewInstance() any {
	m.currentObject = reflect.New(m.valType).Interface()
	return m.currentObject
}

// Table returns model database table name
func (m *Model) Table() string {
	return m.table
}

// FieldByName trying to find field by name and returns the *Field or error
func (m *Model) FieldByName(name string) (*Field, bool) {
	v, ok := m.fieldByAnyName[name]
	return v, ok
}

// Fields returns the slice of *Field containing all model fields
func (m *Model) Fields() []*Field {
	return m.fields
}
