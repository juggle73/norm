package norm

import "strings"

type OptionType int

const (
	ExcludeOption OptionType = iota
	FieldsOption
	ReturningOption
	PrefixOption
	AddTargetsOption
)

type Option interface {
	Type() OptionType
	Value() any
}

type (
	excludeOption    string
	fieldsOption     string
	returningOption  string
	prefixOption     string
	addTargetsOption []any
)

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

func (opt addTargetsOption) Type() OptionType { return AddTargetsOption }
func (opt addTargetsOption) Value() any       { return []any(opt) }
func AddTargets(targets ...any) Option {
	return addTargetsOption(targets)
}

type ComposedOptions struct {
	Exclude    []string
	Fields     []string
	Returning  []string
	Prefix     string
	AddTargets []any
}

func ComposeOptions(opts ...Option) ComposedOptions {
	res := ComposedOptions{
		Exclude:    nil,
		Fields:     nil,
		Prefix:     "",
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
		case addTargetsOption:
			res.AddTargets = opt
		}
	}

	return res
}
