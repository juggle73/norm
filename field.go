package norm

import "reflect"

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
