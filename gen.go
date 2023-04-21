package norm

import (
	"fmt"
	"github.com/iancoleman/strcase"
)

//select column_name, is_nullable, data_type, character_maximum_length, numeric_precision from information_schema.columns where table_name='matches' order by ordinal_position;

type Col struct {
	Name       string `json:"name"`
	IsNullable string
	DataType   string
	Pk         bool
}

func (norm *Norm) Gen(packageName, structName string, cols []Col) string {
	imports := make(map[string]bool)
	structStr := fmt.Sprintf("type %s struct {\n", structName)

	for _, col := range cols {
		pointerPrefix := ""
		if col.IsNullable == "YES" {
			pointerPrefix = "*"
		}
		normTag := ""
		if col.Pk {
			normTag = " norm=\"pk\""
		}
		goType := ""
		switch col.DataType {
		case "bigint":
			goType = "int64"
		case "integer":
			goType = "int"
		case "character varying":
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
