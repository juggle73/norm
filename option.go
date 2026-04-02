package norm

import (
	"fmt"
	"strings"
)

// OptionType identifies the kind of an [Option].
type OptionType int

const (
	ExcludeOption    OptionType = iota // Exclude fields by db name
	FieldsOption                       // Include only specified fields
	ReturningOption                    // RETURNING clause fields
	PrefixOption                       // Table alias prefix for field names
	WhereOption                        // WHERE clause with ? placeholders
	AddTargetsOption                   // Extra scan targets for Pointers
	OffsetOption                       // OFFSET value
	LimitOption                        // LIMIT value
	OrderByOption                      // ORDER BY clause
)

// Option is a functional option for customizing query building methods.
// Use the constructor functions ([Exclude], [Fields], [Where], etc.) to
// create options.
type Option interface {
	Type() OptionType
	Value() any
}

type (
	excludeOption   string
	fieldsOption    string
	returningOption string
	prefixOption    string
	whereOption     struct {
		template string // original where with "?" placeholders
		Args     []any
	}
	addTargetsOption []any
	offsetOption     int
	limitOption      int
	orderByOption    string
)

// parseWhere creates a whereOption from a template and args.
// Returns nil if where is empty.
func parseWhere(where string, args ...any) *whereOption {
	if where == "" {
		return nil
	}

	return &whereOption{
		template: where,
		Args:     args,
	}
}

// Build renders the WHERE clause, replacing each "?" with "$N" starting
// from startBind. Returns the rendered string and the next bind number.
func (w *whereOption) Build(startBind int) (string, int) {
	result := w.template
	bind := startBind
	count := strings.Count(result, "?")
	for i := 0; i < count; i++ {
		result = strings.Replace(result, "?", fmt.Sprintf("$%d", bind), 1)
		bind++
	}
	return result, bind
}

func (opt excludeOption) Type() OptionType { return ExcludeOption }
func (opt excludeOption) Value() any       { return string(opt) }

// Exclude creates an option that excludes the named fields (comma-separated
// db column names) from the result.
//
//	m.Fields(norm.Exclude("id,password"))
func Exclude(fields string) Option {
	return excludeOption(fields)
}

func (opt fieldsOption) Type() OptionType { return FieldsOption }
func (opt fieldsOption) Value() any       { return string(opt) }

// Fields creates an option that includes only the named fields (comma-separated
// db column names).
//
//	m.Fields(norm.Fields("name,email"))
func Fields(fields string) Option {
	return fieldsOption(fields)
}

func (opt returningOption) Type() OptionType { return ReturningOption }
func (opt returningOption) Value() any       { return string(opt) }

// Returning creates an option that adds a RETURNING clause with the given
// field names (comma-separated, any name format).
//
//	m.Insert(norm.Returning("Id"))
func Returning(fields string) Option {
	return returningOption(fields)
}

func (opt prefixOption) Type() OptionType { return PrefixOption }
func (opt prefixOption) Value() any       { return string(opt) }

// Prefix creates an option that adds a table alias prefix to field names.
// Also usable in [BuildConditions] to prefix all column references.
//
//	m.Fields(norm.Prefix("u.")) // "u.id, u.name, u.email"
func Prefix(prefix string) prefixOption {
	return prefixOption(prefix)
}

// BuildWhere renders a WHERE clause string, replacing "?" placeholders with
// "$N" starting from startBind. Returns the rendered string and the args
// slice unchanged. Useful for building UPDATE queries manually.
//
//	set, nextBind := m.UpdateFields(norm.Exclude("id"))
//	whereStr, whereArgs := norm.BuildWhere(nextBind, "id = ?", user.Id)
func BuildWhere(startBind int, where string, args ...any) (string, []any) {
	result := where
	bind := startBind
	count := strings.Count(result, "?")
	for i := 0; i < count; i++ {
		result = strings.Replace(result, "?", fmt.Sprintf("$%d", bind), 1)
		bind++
	}
	return result, args
}

func (opt *whereOption) Type() OptionType { return WhereOption }
func (opt *whereOption) Value() any       { return opt }

// Where creates an option that adds a WHERE clause. Use "?" as placeholders
// for positional arguments — they are replaced with $1, $2, etc. at build time.
//
//	m.Select(norm.Where("name = ? AND age > ?", "John", 18))
func Where(where string, args ...any) Option {
	return parseWhere(where, args...)
}

func (opt addTargetsOption) Type() OptionType { return AddTargetsOption }
func (opt addTargetsOption) Value() any       { return []any(opt) }

// AddTargets creates an option that appends extra scan targets to the
// pointers returned by [Model.Pointers]. Useful for scanning computed
// columns not present in the struct.
//
//	var count int
//	ptrs := m.Pointers(norm.AddTargets(&count))
func AddTargets(targets ...any) Option {
	return addTargetsOption(targets)
}

func (opt offsetOption) Type() OptionType { return OffsetOption }
func (opt offsetOption) Value() any       { return int(opt) }

// Offset creates an option that adds an OFFSET clause to a SELECT query.
//
//	m.Select(norm.Offset(20))
func Offset(offset int) Option {
	return offsetOption(offset)
}

func (opt limitOption) Type() OptionType { return LimitOption }
func (opt limitOption) Value() any       { return int(opt) }

// Limit creates an option that adds a LIMIT clause to a SELECT query.
//
//	m.Select(norm.Limit(10))
func Limit(limit int) Option {
	return limitOption(limit)
}

func (opt orderByOption) Type() OptionType { return OrderByOption }
func (opt orderByOption) Value() any       { return string(opt) }

// Order creates an option that adds an ORDER BY clause to a SELECT query.
// Field names are validated against the model and converted to column names.
//
//	m.Select(norm.Order("Name DESC"))
func Order(orderBy string) Option {
	return orderByOption(orderBy)
}

// ComposedOptions holds the parsed result of all options passed to a method.
type ComposedOptions struct {
	Exclude    []string
	Fields     []string
	Returning  []string
	Prefix     string
	Where      *whereOption
	AddTargets []any
	Offset     int
	Limit      int
	OrderBy    string
}

// ComposeOptions parses a list of [Option] values into a single [ComposedOptions].
func ComposeOptions(opts ...Option) ComposedOptions {
	res := ComposedOptions{
		Exclude:    nil,
		Fields:     nil,
		Prefix:     "",
		Where:      nil,
		AddTargets: nil,
	}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case excludeOption:
			str := string(opt)
			res.Exclude = strings.Split(str, ",")
		case fieldsOption:
			str := string(opt)
			res.Fields = strings.Split(str, ",")
		case returningOption:
			str := string(opt)
			res.Returning = strings.Split(str, ",")
		case prefixOption:
			res.Prefix = string(opt)
		case *whereOption:
			res.Where = opt
		case addTargetsOption:
			res.AddTargets = opt
		case offsetOption:
			res.Offset = int(opt)
		case limitOption:
			res.Limit = int(opt)
		case orderByOption:
			res.OrderBy = string(opt)
		}
	}

	return res
}
