package norm

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/iancoleman/strcase"
)

// isPointerToStruct reports whether v is a pointer to a struct.
func isPointerToStruct(v reflect.Value) bool {
	return v.Kind() == reflect.Pointer && v.Elem().Kind() == reflect.Struct
}

// has reports whether the string slice a contains val (after trimming spaces).
func has(a []string, val string) bool {
	for i := range a {
		if strings.TrimSpace(a[i]) == val {
			return true
		}
	}
	return false
}

// Binds generates a bind placeholder string in "$1, $2, ..." format
// for the given number of parameters.
//
//	norm.Binds(3) // "$1, $2, $3"
//	norm.Binds(0) // ""
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

// parseNormTag parses the "norm" struct tag and returns a map of key-value
// pairs. Returns (nil, false) if the field should be skipped (tag is "-").
// If no tag is present, returns a map with only "dbName" set to the
// snake_case of the field name.
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

// isValidIdentifier reports whether name contains only [a-zA-Z0-9_]
// and is not empty. Used to validate table and column names against
// SQL injection.
func isValidIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// indirectType follows pointer types until it reaches a non-pointer type.
func indirectType(v reflect.Type) reflect.Type {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return v
}
