package jorm

import (
	"fmt"
	"github.com/iancoleman/strcase"
	"reflect"
	"strings"
)

func isPointerToStruct(v reflect.Value) bool {
	return v.Kind() == reflect.Pointer && v.Elem().Kind() == reflect.Struct
}

func has(a []string, val string) int {
	for i := range a {
		if strings.TrimSpace(a[i]) == val {
			return i
		}
	}
	return -1
}

func binds(count int) string {
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

func indirectType(v reflect.Type) reflect.Type {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return v
}
