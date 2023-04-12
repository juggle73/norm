package norm

import (
	"fmt"
	"github.com/iancoleman/strcase"
	"os"
)

//select column_name, is_nullable, data_type, character_maximum_length, numeric_precision from information_schema.columns where table_name='matches' order by ordinal_position;

type Col struct {
	Name                   string `json:"name"`
	IsNullable             string
	DataType               string
	CharacterMaximumLength int
	NumericPrecision       int
}

func (norm *Norm) Gen(structName string, cols []Col, outFile string) error {
	str := fmt.Sprintf(`type %s struct {
`, structName)

	for _, col := range cols {
		pointerPrefix := ""
		if col.IsNullable == "YES" {
			pointerPrefix = "*"
		}
		switch col.DataType {
		case "bigint":
			str += fmt.Sprintf("\t%s %s%s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), pointerPrefix, "int64", strcase.ToLowerCamel(col.Name))
		case "integer":
			str += fmt.Sprintf("\t%s %s%s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), pointerPrefix, "int", strcase.ToLowerCamel(col.Name))
		case "character varying":
			str += fmt.Sprintf("\t%s %s%s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), pointerPrefix, "string", strcase.ToLowerCamel(col.Name))
		case "timestamp with time zone", "date":
			str += fmt.Sprintf("\t%s %s%s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), pointerPrefix, "time.Time", strcase.ToLowerCamel(col.Name))
		case "json", "jsonb":
			str += fmt.Sprintf("\t%s %s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), "map[string]any", strcase.ToLowerCamel(col.Name))
		case "bytea":
			str += fmt.Sprintf("\t%s %s `json:\"%s\"`\n",
				strcase.ToCamel(col.Name), "[]byte]", strcase.ToLowerCamel(col.Name))
		}
	}

	str += "}"

	return os.WriteFile(outFile, []byte(str), os.ModePerm)
}
