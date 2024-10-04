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
//
// Parameters:
//
//	prefix - prefix for field names, table alias, used when using joins in sql query
//	obj.key - field name or [field name]->>[json key name] for json field types
//	obj.value:
//	  for string field type:
//	    string - add [field]=[value] condition
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
	var res []string

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
	case reflect.Map:
		if b.suffix != "" {
			// TODO: add support for json field types
			res = b.stringCondition(val)
		}
	default:

	}

	if len(res) > 0 {
		b.conditions = append(b.conditions, res...)
	}
}

func (b *conditionBuilder) stringCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.String:
		b.values = append(b.values, val.Interface())
		res = append(res, fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values)))
	case reflect.Map:
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return nil
			}
			switch k.String() {
			case "like":
				v := val.MapIndex(k)
				b.values = append(b.values, v.Elem().Interface())
				res = append(res, fmt.Sprintf("%s%s%s LIKE $%d", b.prefix, b.field.dbName, b.suffix, len(b.values)))
			case "isNull":
				v := val.MapIndex(k)
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						res = append(res, fmt.Sprintf("%s%s%s IS NULL", b.prefix, b.field.dbName, b.suffix))
					} else {
						res = append(res, fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix))
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
		res = append(res, fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", ")))
	}

	return res
}

func (b *conditionBuilder) intCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.values = append(b.values, val.Interface())
		res = append(res, fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values)))
	case reflect.Map:
		localRes := ""
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return nil
			}

			v := val.MapIndex(k)

			switch k.String() {
			case "gt", "gte", "lt", "lte", "ne":
				switch v.Elem().Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if localRes != "" {
						localRes += " AND "
					}
					b.values = append(b.values, v.Elem().Interface())
					localRes = fmt.Sprintf("%s%s%s%s %s $%d", localRes, b.prefix, b.field.dbName, b.suffix, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						localRes = fmt.Sprintf("%s%s%s%s IS NULL", localRes, b.prefix, b.field.dbName, b.suffix)
					} else {
						localRes = fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix)
					}
				}
			}
		}
		res = append(res, localRes)
	case reflect.Slice:
		l := val.Len()
		a := make([]string, l)
		for i := 0; i < l; i++ {
			b.values = append(b.values, val.Index(i).Interface())
			a[i] = fmt.Sprintf("$%d", len(b.values))
		}
		res = append(res, fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", ")))
	}

	return res
}

func (b *conditionBuilder) timeCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.String:
		b.values = append(b.values, val.Interface())
		res = append(res, fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values)))
	case reflect.Map:
		localRes := ""
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return nil
			}

			v := val.MapIndex(k)

			switch k.String() {
			case "gt", "gte", "lt", "lte", "ne":
				switch v.Elem().Kind() {
				case reflect.String:
					if localRes != "" {
						localRes += " AND "
					}
					b.values = append(b.values, v.Elem().Interface())
					localRes = fmt.Sprintf("%s%s%s%s %s $%d", localRes, b.prefix, b.field.dbName, b.suffix, operationMap[k.String()], len(b.values))
				}
			case "isNull":
				if v.Elem().Kind() == reflect.Bool {
					if v.Elem().Bool() {
						localRes = fmt.Sprintf("%s%s%s%s IS NULL", localRes, b.prefix, b.field.dbName, b.suffix)
					} else {
						localRes = fmt.Sprintf("%s%s%s IS NOT NULL", b.prefix, b.field.dbName, b.suffix)
					}
				}
			}
		}
		res = append(res, localRes)
	case reflect.Slice:
		l := val.Len()
		a := make([]string, l)
		for i := 0; i < l; i++ {
			b.values = append(b.values, val.Index(i).Interface())
			a[i] = fmt.Sprintf("$%d", len(b.values))
		}
		res = append(res, fmt.Sprintf("%s%s%s IN (%s)", b.prefix, b.field.dbName, b.suffix, strings.Join(a, ", ")))
	}

	return res
}
