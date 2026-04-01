package norm

import (
	"reflect"
	"testing"
)

func TestIsPointerToStruct(t *testing.T) {
	type S struct{}

	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"pointer to struct", &S{}, true},
		{"struct value", S{}, false},
		{"pointer to int", new(int), false},
		{"int value", 42, false},
		{"string", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPointerToStruct(reflect.ValueOf(tt.val))
			if got != tt.want {
				t.Errorf("isPointerToStruct(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestHas(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		val   string
		want  bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"nil slice", nil, "a", false},
		{"with spaces", []string{" a ", " b "}, "a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := has(tt.slice, tt.val)
			if got != tt.want {
				t.Errorf("has(%v, %q) = %v, want %v", tt.slice, tt.val, got, tt.want)
			}
		})
	}
}

func TestBindsFunc(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, ""},
		{1, "$1"},
		{3, "$1, $2, $3"},
		{5, "$1, $2, $3, $4, $5"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := Binds(tt.count)
			if got != tt.want {
				t.Errorf("Binds(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestParseNormTag(t *testing.T) {
	type WithTags struct {
		Id     int    `norm:"pk"`
		Name   string `norm:"dbName=full_name,notnull"`
		Age    int
		Skip   string `norm:"-"`
		Multi  string `norm:"notnull,unique,default=hello"`
		NoTag  string
		Custom int    `norm:"dbType=bigint"`
	}

	typ := reflect.TypeOf(WithTags{})

	t.Run("pk tag", func(t *testing.T) {
		f, _ := typ.FieldByName("Id")
		tags, ok := parseNormTag(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if _, hasPk := tags["pk"]; !hasPk {
			t.Error("expected pk tag")
		}
		if tags["dbName"] != "id" {
			t.Errorf("expected dbName=id, got %q", tags["dbName"])
		}
	})

	t.Run("custom dbName", func(t *testing.T) {
		f, _ := typ.FieldByName("Name")
		tags, ok := parseNormTag(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if tags["dbName"] != "full_name" {
			t.Errorf("expected dbName=full_name, got %q", tags["dbName"])
		}
		if _, has := tags["notnull"]; !has {
			t.Error("expected notnull tag")
		}
	})

	t.Run("no norm tag defaults to snake_case", func(t *testing.T) {
		f, _ := typ.FieldByName("NoTag")
		tags, ok := parseNormTag(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if tags["dbName"] != "no_tag" {
			t.Errorf("expected dbName=no_tag, got %q", tags["dbName"])
		}
	})

	t.Run("skip tag", func(t *testing.T) {
		f, _ := typ.FieldByName("Skip")
		_, ok := parseNormTag(f)
		if ok {
			t.Error("expected ok=false for norm:\"-\"")
		}
	})

	t.Run("multiple tags", func(t *testing.T) {
		f, _ := typ.FieldByName("Multi")
		tags, ok := parseNormTag(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if _, has := tags["notnull"]; !has {
			t.Error("expected notnull")
		}
		if _, has := tags["unique"]; !has {
			t.Error("expected unique")
		}
		if tags["default"] != "hello" {
			t.Errorf("expected default=hello, got %q", tags["default"])
		}
	})
}

func TestIndirectType(t *testing.T) {
	var i int
	var pi *int
	var ppi **int

	typ := reflect.TypeOf(i)
	pTyp := reflect.TypeOf(pi)
	ppTyp := reflect.TypeOf(ppi)

	if got := indirectType(typ); got != typ {
		t.Errorf("indirectType(int) = %v, want %v", got, typ)
	}
	if got := indirectType(pTyp); got != typ {
		t.Errorf("indirectType(*int) = %v, want %v", got, typ)
	}
	if got := indirectType(ppTyp); got != typ {
		t.Errorf("indirectType(**int) = %v, want %v", got, typ)
	}
}
