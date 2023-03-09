package jorm

import (
	"errors"
	"fmt"
	"github.com/iancoleman/strcase"
	"log"
	"reflect"
	"strings"
)

type Model struct {
	valType reflect.Type
	fields  []*Field

	fieldByJSONName map[string]*Field
	fieldByDbName   map[string]*Field
}

func NewModel() *Model {
	return &Model{}
}

func (m *Model) Parse(obj any) error {
	val := reflect.Indirect(reflect.ValueOf(obj))
	if val.Kind() != reflect.Struct {
		return errors.New("object must be a struct or pointer to struct")
	}

	m.fields = make([]*Field, 0)
	m.fieldByJSONName = make(map[string]*Field)
	m.fieldByDbName = make(map[string]*Field)

	m.valType = val.Type()

	c := val.NumField()
	for i := 0; i < c; i++ {
		f := val.Type().Field(i)

		tagValues, ok := parseOrmTag(f)
		if !ok {
			continue
		}

		field := &Field{
			valType:   f.Type,
			name:      f.Name,
			dbName:    tagValues[0],
			tagValues: tagValues,
		}

		m.fields = append(m.fields, field)
		m.fieldByJSONName[strcase.ToLowerCamel(field.name)] = field
		m.fieldByDbName[field.dbName] = field
	}

	return nil
}

func (m *Model) FieldDbNames(exclude, prefix string) []string {
	excludeArr := strings.Split(exclude, ",")

	res := make([]string, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) >= 0 {
			continue
		}

		res = append(res, prefix+f.dbName)
	}

	return res
}

func (m *Model) FieldDbNamesWithBinds(exclude string) []string {
	excludeArr := strings.Split(exclude, ",")

	bind := 1
	res := make([]string, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) >= 0 {
			continue
		}

		res = append(res, fmt.Sprintf("%s=$%d", f.dbName, bind))
		bind++
	}

	return res
}

func (m *Model) FieldPointers(obj any, exclude string) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		log.Fatal("FieldPointers: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) >= 0 {
			continue
		}

		res = append(res, val.FieldByName(f.name).Addr().Interface())
	}

	return res
}

func (m *Model) FieldValues(obj any, exclude string) []any {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		log.Fatal("FieldValues: object must be a pointer to struct")
	}

	val = val.Elem()

	excludeArr := strings.Split(exclude, ",")

	res := make([]any, 0)
	for _, f := range m.fields {
		if has(excludeArr, f.dbName) >= 0 {
			continue
		}

		res = append(res, val.FieldByName(f.name).Interface())
	}

	return res
}
