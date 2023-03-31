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
