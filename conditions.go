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
}

// BuildConditions builds sql WHERE conditions and return them with bind values
func (m *Model) BuildConditions(obj map[string]any) ([]string, []any) {

	builder := conditionBuilder{
		conditions: make([]string, 0),
		values:     make([]any, 0),
	}

	for k, v := range obj {
		mField, ok := m.fieldByAnyName[k]
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
		res = fmt.Sprintf("%s=$%d", b.field.dbName, len(b.values))
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
				res = fmt.Sprintf("%s LIKE $%d", b.field.dbName, len(b.values))
			case "isNull":
				v := val.MapIndex(k)
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s IS NULL", b.field.dbName)
					} else {
						res = fmt.Sprintf("%s IS NOT NULL", b.field.dbName)
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
		res = fmt.Sprintf("%s IN (%s)", b.field.dbName, strings.Join(a, ", "))
	}

	return res
}

func (b *conditionBuilder) intCondition(val reflect.Value) string {
	res := ""

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.values = append(b.values, val.Interface())
		res = fmt.Sprintf("%s=$%d", b.field.dbName, len(b.values))
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
					res = fmt.Sprintf("%s%s %s $%d", res, b.field.dbName, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s%s IS NULL", res, b.field.dbName)
					} else {
						res = fmt.Sprintf("%s IS NOT NULL", b.field.dbName)
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
		res = fmt.Sprintf("%s IN (%s)", b.field.dbName, strings.Join(a, ", "))
	}

	return res
}

func (b *conditionBuilder) timeCondition(val reflect.Value) string {
	res := ""

	switch val.Kind() {
	case reflect.String:
		b.values = append(b.values, val.Interface())
		res = fmt.Sprintf("%s=$%d", b.field.dbName, len(b.values))
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
					res = fmt.Sprintf("%s%s %s $%d", res, b.field.dbName, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = fmt.Sprintf("%s%s IS NULL", res, b.field.dbName)
					} else {
						res = fmt.Sprintf("%s IS NOT NULL", b.field.dbName)
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
		res = fmt.Sprintf("%s IN (%s)", b.field.dbName, strings.Join(a, ", "))
	}

	return res
}
