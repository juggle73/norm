package norm

import (
	"fmt"
	"github.com/iancoleman/strcase"
	"reflect"
	"strings"
)

// isPointerToStruct checks is reflect.Value is pointer to struct
func isPointerToStruct(v reflect.Value) bool {
	return v.Kind() == reflect.Pointer && v.Elem().Kind() == reflect.Struct
}

// has checks is there a value val in slice a
func has(a []string, val string) bool {
	for i := range a {
		if strings.TrimSpace(a[i]) == val {
			return true
		}
	}
	return false
}

// Binds generates sql binds string in "$1, $2, ..." format
func Binds(count int) string {
	if count == 0 {
		return ""
	}
	str := "$1"
	for i := 1; i < count; i++ {
		str = fmt.Sprintf("%s, $%d", str, i+1)
	}

	return str
}

func parseNormTag(field reflect.StructField) (map[string]string, bool) {
	res := make(map[string]string)

	ormTag, ok := field.Tag.Lookup("norm")
	ormTag = strings.TrimSpace(ormTag)
	if ormTag == "-" {
		return nil, false
	}
	if !ok || ormTag == "" {
		res["dbName"] = strcase.ToSnake(field.Name)
		return res, true
	}

	entries := strings.Split(ormTag, ",")
	for i := range entries {
		kv := strings.Split(entries[i], "=")
		if len(kv) == 1 {
			res[entries[i]] = ""
		} else if len(kv) == 2 {
			res[kv[0]] = kv[1]
		}
	}

	// Add default name if not exist
	if _, ok := res["dbName"]; !ok {
		res["dbName"] = strcase.ToSnake(field.Name)
	}

	return res, true
}

// indirectType returns the type that v points to.
func indirectType(v reflect.Type) reflect.Type {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return v
}
