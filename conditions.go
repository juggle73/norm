package norm

import (
	"fmt"
	"reflect"
	"strings"
)

var operationMap = map[string]string{
	"gt":  ">",
	"gte": ">=",
	"lt":  "<",
	"lte": "<=",
	"ne":  "!=",
}

type conditionBuilder struct {
	conditions []string
	values     []any
	field      *Field
	suffix     string
	prefix     string
}

// BuildConditions builds sql WHERE conditions and return them with bind values
func (m *Model) BuildConditions(obj map[string]any, prefix string) ([]string, []any) {

	builder := conditionBuilder{
		conditions: make([]string, 0),
		values:     make([]any, 0),
		prefix:     prefix,
	}

	for k, v := range obj {

		builder.suffix = ""
		fieldName := k
		parts := strings.Split(fieldName, "->>")
		if len(parts) == 2 {
			fieldName = parts[0]
			builder.suffix = "->>" + parts[1]
		}
		mField, ok := m.fieldByAnyName[fieldName]
		if !ok {
			continue
		}

		val := reflect.ValueOf(v)
		builder.field = mField
		builder.getCondition(val)
	}

	return builder.conditions, builder.values
}

func (b *conditionBuilder) getCondition(val reflect.Value) {
	res := ""

	vType := indirectType(b.field.valType)

	fmt.Println("vType.Kind().String():", vType.Kind().String())
	fmt.Println("b.field.valType.String()", b.field.valType.String())

	switch vType.Kind() {
	case reflect.String:
		res = b.stringCondition(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		res = b.intCondition(val)
	case reflect.Struct:
		if b.field.valType.String() == "time.Time" || b.field.valType.String() == "pgtype.Timestamptz" {
			res = b.timeCondition(val)
		}
	default:

	}

	if res != "" {
		b.conditions = append(b.conditions, res)
	}
}

func (b *conditionBuilder) stringCondition(val reflect.Value) string {
	res := ""

	switch val.Kind() {
	case reflect.String:
		b.values = append(b.values, val.Interface())
		res = fmt.Sprintf("%s%s=$%d", b.field.dbName, b.suffix, len(b.values))
	case reflect.Map:
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return ""
			}
			switch k.String() {
			case "like":
				v := val.MapIndex(k)
				b.values = append(b.values, v.Elem().Interface())
				res = fmt.Sprintf("%s%s%s LIKE $%d", b.prefix, b.field.dbName, b.suffix, len(b.values))
			case "isNull":
				v := val.MapIndex(k)
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s%s%s IS NULL", b.prefix, b.field.dbName, b.suffix)
					} else {
						res = fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix)
					}
				}
			}
		}
	case reflect.Slice:
		l := val.Len()
		a := make([]string, l)
		for i := 0; i < l; i++ {
			b.values = append(b.values, val.Index(i).Interface())
			a[i] = fmt.Sprintf("$%d", len(b.values))
		}
		res = fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", "))
	}

	return res
}

func (b *conditionBuilder) intCondition(val reflect.Value) string {
	res := ""

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.values = append(b.values, val.Interface())
		res = fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values))
	case reflect.Map:
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return ""
			}

			v := val.MapIndex(k)

			switch k.String() {
			case "gt", "gte", "lt", "lte", "ne":
				switch v.Elem().Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if res != "" {
						res += " AND "
					}
					b.values = append(b.values, v.Elem().Interface())
					res = fmt.Sprintf("%s%s%s%s %s $%d", res, b.prefix, b.field.dbName, b.suffix, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s%s%s%s IS NULL", res, b.prefix, b.field.dbName, b.suffix)
					} else {
						res = fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix)
					}
				}
			}
		}
	case reflect.Slice:
		l := val.Len()
		a := make([]string, l)
		for i := 0; i < l; i++ {
			b.values = append(b.values, val.Index(i).Interface())
			a[i] = fmt.Sprintf("$%d", len(b.values))
		}
		res = fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", "))
	}

	return res
}

func (b *conditionBuilder) timeCondition(val reflect.Value) string {
	res := ""

	switch val.Kind() {
	case reflect.String:
		b.values = append(b.values, val.Interface())
		res = fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values))
	case reflect.Map:
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return ""
			}

			v := val.MapIndex(k)

			switch k.String() {
			case "gt", "gte", "lt", "lte", "ne":
				switch v.Elem().Kind() {
				case reflect.String:
					if res != "" {
						res += " AND "
					}
					b.values = append(b.values, v.Elem().Interface())
					res = fmt.Sprintf("%s%s%s%s %s $%d", res, b.prefix, b.field.dbName, b.suffix, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s%s%s%s IS NULL", res, b.prefix, b.field.dbName, b.suffix)
					} else {
						res = fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix)
					}
				}
			}
		}
	case reflect.Slice:
		l := val.Len()
		a := make([]string, l)
		for i := 0; i < l; i++ {
			b.values = append(b.values, val.Index(i).Interface())
			a[i] = fmt.Sprintf("$%d", len(b.values))
		}
		res = fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", "))
	}

	return res
}
