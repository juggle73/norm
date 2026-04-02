package norm

import (
	"fmt"
	"reflect"
	"time"

	"github.com/iancoleman/strcase"
)

// Field holds metadata about a single struct field: its Go name, database
// column name, type, and parsed tag values.
type Field struct {
	valType   reflect.Type
	name      string
	dbName    string
	tagValues map[string]string
}

// hasTag reports whether the field has the given tag key in its norm tag.
func (f *Field) hasTag(tag string) bool {
	_, ok := f.tagValues[tag]
	return ok
}

// Tag returns the value of a norm tag key and whether it exists.
//
//	val, ok := field.Tag("default") // val="0", ok=true for `norm:"default=0"`
func (f *Field) Tag(tag string) (string, bool) {
	val, ok := f.tagValues[tag]
	return val, ok
}

// DbName returns the database column name (snake_case).
func (f *Field) DbName() string {
	return f.dbName
}

// Name returns the Go struct field name.
func (f *Field) Name() string {
	return f.name
}

// JsonName returns the field name in lowerCamelCase, suitable for JSON keys.
func (f *Field) JsonName() string {
	return strcase.ToLowerCamel(f.name)
}

// Type returns the reflect.Type of the struct field.
func (f *Field) Type() reflect.Type {
	return f.valType
}

// isJSON reports whether the field should be marshaled/unmarshaled as JSON.
// A field is JSON if its underlying type is a struct (but not time.Time).
// Maps and slices are excluded — database drivers handle them natively.
func (f *Field) isJSON() bool {
	t := indirectType(f.valType)
	return t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Time{})
}

// jsonScanner implements the sql.Scanner interface for JSON struct fields.
// It unmarshals JSON bytes or strings into the target pointer using the
// configured unmarshal function.
type jsonScanner struct {
	target    any
	unmarshal func(data []byte, v any) error
}

// Scan implements the sql.Scanner interface. It accepts []byte, string,
// or nil as input and unmarshals JSON into the target.
func (s *jsonScanner) Scan(src any) error {
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	case nil:
		return nil
	default:
		return fmt.Errorf("jsonScanner: unsupported source type %T", src)
	}
	return s.unmarshal(data, s.target)
}
