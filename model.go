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
	table   string
	valType reflect.Type
	fields  []*Field
	//fieldByJSONName map[string]*Field
	//fieldByDbName   map[string]*Field
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
	//m.fieldByJSONName = make(map[string]*Field)
	//m.fieldByDbName = make(map[string]*Field)
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

// Pointers returns slice of field pointers for obj, excluding specified in the parameter exclude
func (m *Model) Pointers(exclude string) []any {
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

	return res
}

// Values returns slice of field values as interface{} for obj, excluding specified in the parameter exclude
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
