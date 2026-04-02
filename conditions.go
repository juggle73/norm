package norm

import (
	"fmt"
	"reflect"
	"strings"
)

// operationMap maps condition operator names to SQL operators.
var operationMap = map[string]string{
	"gt":  ">",
	"gte": ">=",
	"lt":  "<",
	"lte": "<=",
	"ne":  "!=",
}

// conditionBuilder accumulates WHERE conditions and bind values
// while processing a single [BuildConditions] call.
type conditionBuilder struct {
	conditions []string
	values     []any
	field      *Field
	suffix     string
	prefix     string
}

// BuildConditions builds SQL WHERE conditions from a map of field names
// to filter values. Returns a slice of condition strings and a slice of
// bind values.
//
// The prefix parameter adds a table alias to all field references (e.g. "u."
// for JOINs). Map keys are field names (any format) or "field->>jsonKey"
// for JSON field access.
//
// Supported value types:
//   - Direct value (string, int, float, bool): equality condition
//   - map[string]any with operators: {"gt": 18, "lte": 65, "like": "%john%",
//     "isNull": true}
//   - []any: IN clause
//
//	conds, vals := m.BuildConditions(map[string]any{
//	    "name": "John",
//	    "age":  map[string]any{"gte": 18, "lt": 65},
//	}, "")
func (m *modelMeta) BuildConditions(obj map[string]any, prefix string) ([]string, []any) {

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

// getCondition dispatches to the appropriate type-specific condition builder.
func (b *conditionBuilder) getCondition(val reflect.Value) {
	var res []string

	vType := indirectType(b.field.valType)

	switch vType.Kind() {
	case reflect.String:
		res = b.stringCondition(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		res = b.intCondition(val)
	case reflect.Float32, reflect.Float64:
		res = b.floatCondition(val)
	case reflect.Bool:
		res = b.boolCondition(val)
	case reflect.Struct:
		if b.field.valType.String() == "time.Time" {
			res = b.timeCondition(val)
		}
	case reflect.Map:
		if b.suffix != "" {
			res = b.stringCondition(val)
		}
	}

	if len(res) > 0 {
		b.conditions = append(b.conditions, res...)
	}
}

// stringCondition builds conditions for string-typed fields.
// Supports equality, LIKE, IS NULL, and IN operators.
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

// intCondition builds conditions for integer-typed fields (all int and uint sizes).
// Supports equality, comparison operators (gt, gte, lt, lte, ne), IS NULL, and IN.
func (b *conditionBuilder) intCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
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
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
					reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
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

// boolCondition builds conditions for boolean-typed fields.
// Supports equality and IS NULL.
func (b *conditionBuilder) boolCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.Bool:
		b.values = append(b.values, val.Interface())
		res = append(res, fmt.Sprintf("%s%s%s=$%d", b.prefix, b.field.dbName, b.suffix, len(b.values)))
	case reflect.Map:
		keys := val.MapKeys()
		for _, k := range keys {
			if k.Kind() != reflect.String {
				return nil
			}
			if k.String() == "isNull" {
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
	}

	return res
}

// floatCondition builds conditions for float-typed fields (float32, float64).
// Supports equality, comparison operators (gt, gte, lt, lte, ne), IS NULL, and IN.
func (b *conditionBuilder) floatCondition(val reflect.Value) []string {
	res := make([]string, 0)

	switch val.Kind() {
	case reflect.Float32, reflect.Float64:
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
				case reflect.Float32, reflect.Float64:
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
		if localRes != "" {
			res = append(res, localRes)
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

// timeCondition builds conditions for time.Time fields.
// Time values are passed as strings. Supports equality, comparison operators
// (gt, gte, lt, lte, ne), IS NULL, and IN.
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
