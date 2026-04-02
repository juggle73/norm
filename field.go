package norm

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/iancoleman/strcase"
)

// Field struct representing struct field reflect attributes
type Field struct {
	valType   reflect.Type
	name      string
	dbName    string
	tagValues map[string]string
}

func (f *Field) hasTag(tag string) bool {
	_, ok := f.tagValues[tag]

	return ok
}

func (f *Field) Tag(tag string) (string, bool) {
	val, ok := f.tagValues[tag]
	return val, ok
}

func (f *Field) DbName() string {
	return f.dbName
}

func (f *Field) Name() string {
	return f.name
}

func (f *Field) JsonName() string {
	return strcase.ToLowerCamel(f.name)
}

func (f *Field) Type() reflect.Type {
	return f.valType
}

// isJSON returns true if the field should be marshaled/unmarshaled as JSON.
// A field is JSON if its underlying type is a struct (but not time.Time).
// Maps and slices are excluded — pgx handles them natively.
func (f *Field) isJSON() bool {
	t := indirectType(f.valType)
	return t.Kind() == reflect.Struct && t != reflect.TypeOf(time.Time{})
}

// jsonScanner implements sql.Scanner for JSON fields.
type jsonScanner struct {
	target any
}

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
	return json.Unmarshal(data, s.target)
}

// jsonValue marshals a value to JSON bytes for writing to DB.
func jsonValue(v any) ([]byte, error) {
	return json.Marshal(v)
}
