package norm

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MapScanner struct {
	classes map[uint32]string
}

// NewMapScanner creates and returns new instance of MapScanner
func NewMapScanner(conn *pgxpool.Pool) (*MapScanner, error) {

	ms := &MapScanner{classes: make(map[uint32]string)}

	rows, err := conn.Query(context.Background(),
		"select oid, relname from pg_class")
	if err != nil {
		return nil, err
	}

	var (
		oid  uint32
		name string
	)

	for rows.Next() {
		err = rows.Scan(&oid, &name)
		if err != nil {
			return nil, err
		}
		ms.classes[oid] = name
	}

	return ms, nil
}

func (ms *MapScanner) Scan(rows pgx.Rows) (map[string]any, error) {
	descriptions := rows.FieldDescriptions()
	res := make(map[string]any)

	values, err := rows.Values()
	if err != nil {
		return nil, err
	}

	for i := range descriptions {
		tableName, ok := ms.classes[descriptions[i].TableOID]
		if !ok {
			return nil, errors.New(
				fmt.Sprintf("table for oid %d not found", descriptions[i].TableOID))
		}
		m, ok := res[tableName]
		if !ok {
			m = make(map[string]any)
			res[tableName] = m
		}
		mm := m.(map[string]any)
		mm[descriptions[i].Name] = values[i]
	}

	return res, nil
}
