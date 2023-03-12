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
func has(a []string, val string) int {
	for i := range a {
		if strings.TrimSpace(a[i]) == val {
			return i
		}
	}
	return -1
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

func parseOrmTag(field reflect.StructField) ([]string, bool) {
	res := make([]string, 1)

	ormTag, ok := field.Tag.Lookup("orm")
	ormTag = strings.TrimSpace(ormTag)
	if ormTag == "-" {
		return nil, false
	}
	if !ok || ormTag == "" {
		res[0] = strcase.ToSnake(field.Name)
		return res, true
	}

	entries := strings.Split(ormTag, ",")
	for i := range entries {
		if i == 0 {
			if entries[i] == "" {
				res[0] = strcase.ToSnake(field.Name)
			} else {
				res[0] = entries[i]
			}
		} else {
			res = append(res, entries[i])
		}
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
