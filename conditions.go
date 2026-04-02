package norm

import (
	"fmt"
	"strings"
)

// Cond represents a single WHERE condition for [BuildConditions].
// Use the constructor functions ([Eq], [Gt], [Like], [In], [IsNull], etc.)
// to create conditions.
type Cond interface {
	isCond()
}

// condition represents a comparison condition (=, >, >=, <, <=, !=, LIKE).
type condition struct {
	field string
	op    string
	value any
}

func (c condition) isCond() {}

// condIn represents an IN (...) condition.
type condIn struct {
	field  string
	values []any
}

func (c condIn) isCond() {}

// condIsNull represents IS NULL / IS NOT NULL.
type condIsNull struct {
	field  string
	isNull bool
}

func (c condIsNull) isCond() {}

// prefixOption also implements Cond so Prefix() works in BuildConditions.
func (o prefixOption) isCond() {}

// Eq creates an equality condition: field = value.
//
//	norm.Eq("name", "John")  // name=$1
func Eq(field string, value any) Cond {
	return condition{field: field, op: "=", value: value}
}

// Gt creates a greater-than condition: field > value.
//
//	norm.Gt("age", 18)  // age > $1
func Gt(field string, value any) Cond {
	return condition{field: field, op: ">", value: value}
}

// Gte creates a greater-or-equal condition: field >= value.
//
//	norm.Gte("age", 18)  // age >= $1
func Gte(field string, value any) Cond {
	return condition{field: field, op: ">=", value: value}
}

// Lt creates a less-than condition: field < value.
//
//	norm.Lt("age", 65)  // age < $1
func Lt(field string, value any) Cond {
	return condition{field: field, op: "<", value: value}
}

// Lte creates a less-or-equal condition: field <= value.
//
//	norm.Lte("age", 65)  // age <= $1
func Lte(field string, value any) Cond {
	return condition{field: field, op: "<=", value: value}
}

// Ne creates a not-equal condition: field != value.
//
//	norm.Ne("status", "deleted")  // status != $1
func Ne(field string, value any) Cond {
	return condition{field: field, op: "!=", value: value}
}

// Like creates a LIKE condition: field LIKE value.
//
//	norm.Like("name", "%john%")  // name LIKE $1
func Like(field string, value any) Cond {
	return condition{field: field, op: "LIKE", value: value}
}

// IsNull creates an IS NULL or IS NOT NULL condition.
//
//	norm.IsNull("email", true)   // email IS NULL
//	norm.IsNull("email", false)  // email IS NOT NULL
func IsNull(field string, isNull bool) Cond {
	return condIsNull{field: field, isNull: isNull}
}

// In creates an IN (...) condition.
//
//	norm.In("id", 1, 2, 3)          // id IN ($1, $2, $3)
//	norm.In("name", "Alice", "Bob") // name IN ($1, $2)
func In(field string, values ...any) Cond {
	return condIn{field: field, values: values}
}

// parseFieldSuffix splits "data->>key" into ("data", "->>key").
// Returns (field, "") if no JSON accessor is present.
func parseFieldSuffix(field string) (string, string) {
	parts := strings.Split(field, "->>")
	if len(parts) == 2 {
		return parts[0], "->>" + parts[1]
	}
	return field, ""
}

// BuildConditions builds SQL WHERE conditions from typed [Cond] values.
// Returns a slice of condition strings and a slice of bind values.
//
// Use [Prefix] to add a table alias to all column references (e.g. for JOINs).
// Field names accept any format (struct name, camelCase, snake_case).
// Use "field->>jsonKey" for JSON field access.
//
//	conds, vals := m.BuildConditions(
//	    norm.Eq("name", "John"),
//	    norm.Gte("age", 18),
//	    norm.Lt("age", 65),
//	    norm.In("id", 1, 2, 3),
//	    norm.Like("email", "%@gmail.com"),
//	    norm.IsNull("deleted_at", true),
//	    norm.Prefix("u."),
//	)
func (m *modelMeta) BuildConditions(conds ...Cond) ([]string, []any) {
	var prefix string
	for _, c := range conds {
		if p, ok := c.(prefixOption); ok {
			prefix = string(p)
		}
	}

	var conditions []string
	var values []any

	for _, c := range conds {
		switch v := c.(type) {
		case condition:
			fieldName, suffix := parseFieldSuffix(v.field)
			f, ok := m.fieldByAnyName[fieldName]
			if !ok {
				continue
			}
			values = append(values, v.value)
			dbField := prefix + f.dbName + suffix
			if v.op == "=" {
				conditions = append(conditions, fmt.Sprintf("%s=$%d", dbField, len(values)))
			} else {
				conditions = append(conditions, fmt.Sprintf("%s %s $%d", dbField, v.op, len(values)))
			}

		case condIn:
			fieldName, suffix := parseFieldSuffix(v.field)
			f, ok := m.fieldByAnyName[fieldName]
			if !ok {
				continue
			}
			placeholders := make([]string, len(v.values))
			for i, val := range v.values {
				values = append(values, val)
				placeholders[i] = fmt.Sprintf("$%d", len(values))
			}
			conditions = append(conditions, fmt.Sprintf("%s%s%s IN (%s)",
				prefix, f.dbName, suffix, strings.Join(placeholders, ", ")))

		case condIsNull:
			fieldName, suffix := parseFieldSuffix(v.field)
			f, ok := m.fieldByAnyName[fieldName]
			if !ok {
				continue
			}
			if v.isNull {
				conditions = append(conditions, fmt.Sprintf("%s%s%s IS NULL", prefix, f.dbName, suffix))
			} else {
				conditions = append(conditions, fmt.Sprintf("%s%s%s IS NOT NULL", prefix, f.dbName, suffix))
			}
		}
	}

	return conditions, values
}
