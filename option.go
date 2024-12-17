package norm

import (
	"fmt"
	"strings"
)

type OptionType int

const (
	ExcludeOption OptionType = iota
	FieldsOption
	ReturningOption
	PrefixOption
	WhereOption
	AddTargetsOption
	OffsetOption
	LimitOption
)

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
		WhereString string
		Binds       int
		Args        []any
	}
	addTargetsOption []any
	offsetOption     int
	limitOption      int
)

func parseWhere(where string, args ...any) *whereOption {
	if where == "" {
		return nil
	}

	count := strings.Count(where, "?")
	for bind := 1; bind <= count; bind++ {
		where = strings.Replace(where, "?", fmt.Sprintf("$%d", bind), 1)
	}

	return &whereOption{
		WhereString: where,
		Binds:       count,
		Args:        args,
	}
}

func (opt excludeOption) Type() OptionType { return ExcludeOption }
func (opt excludeOption) Value() any       { return string(opt) }
func Exclude(fields string) Option {
	return excludeOption(fields)
}

func (opt fieldsOption) Type() OptionType { return FieldsOption }
func (opt fieldsOption) Value() any       { return string(opt) }
func Fields(fields string) Option {
	return fieldsOption(fields)
}

func (opt returningOption) Type() OptionType { return ReturningOption }
func (opt returningOption) Value() any       { return string(opt) }
func Returning(fields string) Option {
	return returningOption(fields)
}

func (opt prefixOption) Type() OptionType { return PrefixOption }
func (opt prefixOption) Value() any       { return string(opt) }
func Prefix(prefix string) Option {
	return prefixOption(prefix)
}

func (opt *whereOption) Type() OptionType { return WhereOption }
func (opt *whereOption) Value() any       { return opt }
func Where(where string, args ...any) Option {
	return parseWhere(where, args...)
}

func (opt addTargetsOption) Type() OptionType { return AddTargetsOption }
func (opt addTargetsOption) Value() any       { return []any(opt) }
func AddTargets(targets ...any) Option {
	return addTargetsOption(targets)
}

func (opt offsetOption) Type() OptionType { return OffsetOption }
func (opt offsetOption) Value() any       { return int(opt) }
func Offset(offset int) Option {
	return offsetOption(offset)
}

func (opt limitOption) Type() OptionType { return LimitOption }
func (opt limitOption) Value() any       { return int(opt) }
func Limit(limit int) Option {
	return offsetOption(limit)
}

type ComposedOptions struct {
	Exclude    []string
	Fields     []string
	Returning  []string
	Prefix     string
	Where      *whereOption
	AddTargets []any
	Offset     int
	Limit      int
}

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
		}
	}

	return res
}
