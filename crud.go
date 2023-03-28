package norm

import (
	"fmt"
	"strings"
)

// CreateSQL returns INSERT clause for Model
func (m *Model) CreateSQL(exclude, returning string) string {
	dbNames := m.DbNames(exclude, "")
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		m.table,
		strings.Join(dbNames, ", "),
		Binds(len(dbNames)),
	)

	if returning != "" {
		sql = fmt.Sprintf("%s RETURNING %s", sql, returning)
	}

	return sql
}
