package norm

import (
	"fmt"
	"strings"

	"github.com/iancoleman/strcase"
)

type joinType string

const (
	innerJoin joinType = "INNER JOIN"
	leftJoin  joinType = "LEFT JOIN"
	rightJoin joinType = "RIGHT JOIN"
)

type joinEntry struct {
	jType joinType
	model *Model
	on    string
}

// Join is a query builder for SELECT with JOINs.
// Fields are auto-prefixed with table names. Pointers() collects scan targets
// from all models in order.
type Join struct {
	base    *Model
	joins   []joinEntry
	where   *whereOption
	orderBy string
	limit   int
	offset  int
}

// NewJoin creates a new Join builder with the given base (FROM) model.
func NewJoin(base *Model) *Join {
	return &Join{
		base:  base,
		joins: make([]joinEntry, 0),
	}
}

// Inner adds an INNER JOIN.
func (j *Join) Inner(m *Model, on string) *Join {
	j.joins = append(j.joins, joinEntry{innerJoin, m, on})
	return j
}

// Left adds a LEFT JOIN.
func (j *Join) Left(m *Model, on string) *Join {
	j.joins = append(j.joins, joinEntry{leftJoin, m, on})
	return j
}

// Right adds a RIGHT JOIN.
func (j *Join) Right(m *Model, on string) *Join {
	j.joins = append(j.joins, joinEntry{rightJoin, m, on})
	return j
}

// Where sets the WHERE clause with ? placeholders.
func (j *Join) Where(where string, args ...any) *Join {
	j.where = parseWhere(where, args...)
	return j
}

// Order sets the ORDER BY clause (raw SQL, use table.column format).
func (j *Join) Order(orderBy string) *Join {
	j.orderBy = orderBy
	return j
}

// Limit sets the LIMIT value.
func (j *Join) Limit(limit int) *Join {
	j.limit = limit
	return j
}

// Offset sets the OFFSET value.
func (j *Join) Offset(offset int) *Join {
	j.offset = offset
	return j
}

// Select builds the full SELECT ... FROM ... JOIN ... query.
// Returns SQL string, args, and error.
func (j *Join) Select() (string, []any, error) {
	allFields := j.collectFields(j.base)
	for _, je := range j.joins {
		allFields = append(allFields, j.collectFields(je.model)...)
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(allFields, ", "), j.base.Table())

	for _, je := range j.joins {
		sql += fmt.Sprintf(" %s %s ON %s", je.jType, je.model.Table(), je.on)
	}

	var args []any

	if j.where != nil {
		whereStr, _ := j.where.Build(1)
		sql += " WHERE " + whereStr
		args = append(args, j.where.Args...)
	}

	if j.orderBy != "" {
		sql += " ORDER BY " + j.orderBy
	}

	if j.limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", j.limit)
	}
	if j.offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", j.offset)
	}

	return sql, args, nil
}

// Pointers returns scan targets from all models in order (base + joins).
func (j *Join) Pointers() []any {
	ptrs := j.base.Pointers()
	for _, je := range j.joins {
		ptrs = append(ptrs, je.model.Pointers()...)
	}
	return ptrs
}

// Auto adds an INNER JOIN with the ON clause resolved automatically from FK tags.
// Panics if no FK relationship is found or if the relationship is ambiguous.
func (j *Join) Auto(m *Model) *Join {
	j.joins = append(j.joins, joinEntry{innerJoin, m, j.resolveFK(m)})
	return j
}

// AutoLeft adds a LEFT JOIN with the ON clause resolved automatically from FK tags.
// Panics if no FK relationship is found or if the relationship is ambiguous.
func (j *Join) AutoLeft(m *Model) *Join {
	j.joins = append(j.joins, joinEntry{leftJoin, m, j.resolveFK(m)})
	return j
}

// resolveFK finds the FK relationship between m and existing models in the join.
// Checks both directions: m referencing existing models, and existing models referencing m.
// Panics on no match, ambiguous match, or missing/composite PK.
func (j *Join) resolveFK(m *Model) string {
	existing := make([]*Model, 0, 1+len(j.joins))
	existing = append(existing, j.base)
	for _, je := range j.joins {
		existing = append(existing, je.model)
	}

	var matches []string

	// Direction 1: m has FK pointing to an existing model
	m.mut.RLock()
	for _, f := range m.fields {
		fkRef, hasFk := f.tagValues["fk"]
		if !hasFk {
			continue
		}
		fkTable := strcase.ToSnake(fkRef)
		for _, em := range existing {
			if em.table == fkTable {
				if len(em.pk) != 1 {
					panic(fmt.Sprintf("Auto: referenced model %q must have exactly one PK field", em.table))
				}
				on := fmt.Sprintf("%s.%s = %s.%s", m.table, f.dbName, em.table, em.pk[0])
				matches = append(matches, on)
			}
		}
	}
	m.mut.RUnlock()

	// Direction 2: an existing model has FK pointing to m
	for _, em := range existing {
		em.mut.RLock()
		for _, f := range em.fields {
			fkRef, hasFk := f.tagValues["fk"]
			if !hasFk {
				continue
			}
			fkTable := strcase.ToSnake(fkRef)
			if fkTable == m.table {
				if len(m.pk) != 1 {
					panic(fmt.Sprintf("Auto: referenced model %q must have exactly one PK field", m.table))
				}
				on := fmt.Sprintf("%s.%s = %s.%s", em.table, f.dbName, m.table, m.pk[0])
				matches = append(matches, on)
			}
		}
		em.mut.RUnlock()
	}

	if len(matches) == 0 {
		panic(fmt.Sprintf("Auto: no FK relationship found between %q and existing models", m.table))
	}
	if len(matches) > 1 {
		panic(fmt.Sprintf("Auto: ambiguous FK relationship for %q (%d matches), use Inner/Left/Right instead", m.table, len(matches)))
	}

	return matches[0]
}

// collectFields returns field names prefixed with table name.
func (j *Join) collectFields(m *Model) []string {
	m.mut.RLock()
	defer m.mut.RUnlock()

	res := make([]string, 0, len(m.fields))
	for _, f := range m.fields {
		res = append(res, m.table+"."+f.dbName)
	}
	return res
}
