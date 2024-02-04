package norm

import (
	"context"
	"fmt"
	"github.com/iancoleman/strcase"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
)

//select column_name, is_nullable, data_type, character_maximum_length, numeric_precision from information_schema.columns where table_name='matches' order by ordinal_position;

type Col struct {
	Name       string `json:"name"`
	IsNullable string `json:"isNullable"`
	DataType   string `json:"dataType"`
	Pk         bool   `json:"pk"`
	Fk         string `json:"fk"`
}

func (n *Norm) Gen(packageName, structName string, cols []Col) string {
	imports := make(map[string]bool)
	structStr := fmt.Sprintf("type %s struct {\n", structName)

	for _, col := range cols {
		pointerPrefix := ""
		if col.IsNullable == "YES" {
			pointerPrefix = "*"
		}
		normTag := ""
		if col.Pk {
			normTag = " norm:\"pk\""
		} else if col.Fk != "" {
			normTag = fmt.Sprintf(" norm:\"fk=%s\"", col.Fk)
		}

		goType := ""
		switch col.DataType {
		case "bigint":
			goType = "int64"
		case "integer":
			goType = "int"
		case "character varying", "text":
			goType = "string"
		case "boolean":
			goType = "bool"
		case "time", "timetz", "time with time zone", "timestamp", "timestamptz", "timestamp with time zone", "date":
			goType = "time.Time"
			imports["time"] = true
		case "json", "jsonb":
			goType = "map[string]any"
			pointerPrefix = ""
		case "bytea":
			goType = "[]byte"
			pointerPrefix = ""
		default:
			continue
		}

		structStr += fmt.Sprintf("\t%s %s%s `json:\"%s\"%s`\n",
			strcase.ToCamel(col.Name), pointerPrefix, goType, strcase.ToLowerCamel(col.Name), normTag)
	}

	structStr += "}"

	res := fmt.Sprintf("package %s\n\n", packageName)
	if len(imports) > 0 {
		res += "import (\n"
		for k := range imports {
			res = fmt.Sprintf("%s\"%s\"\n", res, k)
		}
		res += ")\n\n"
	}

	res += structStr

	return res
}

func (n *Norm) GenDb(pool *pgxpool.Pool, packageName, tableName string) string {
	sql := fmt.Sprintf(
		"select column_name, is_nullable, data_type from information_schema.columns where table_name='%s' order by ordinal_position",
		tableName)

	rows, err := pool.Query(context.Background(), sql)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	cols := make([]Col, 0)

	for rows.Next() {
		col := Col{}
		err = rows.Scan(
			&col.Name,
			&col.IsNullable,
			&col.DataType)
		if err != nil {
			log.Fatal(err)
		}
		cols = append(cols, col)
	}

	return n.Gen(packageName, strcase.ToLowerCamel(tableName), cols)
}
