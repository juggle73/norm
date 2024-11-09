package norm

import "strings"

type OptionType int

const (
	ExcludeOption OptionType = iota
	FieldsOption
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

type options struct {
	exclude    []string
	fields     []string
	prefix     string
	addTargets []any
}

func composeOptions(opts ...Option) options {
	res := options{
		exclude:    nil,
		fields:     nil,
		prefix:     "",
		addTargets: nil,
	}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case excludeOption:
			str := string(opt)
			res.exclude = strings.Split(str, ",")
		case fieldsOption:
			str := string(opt)
			res.fields = strings.Split(str, ",")
		case prefixOption:
			res.prefix = string(opt)
		case addTargetsOption:
			res.addTargets = opt
		}
	}

	return res
}
