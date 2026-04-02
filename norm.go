package norm

import (
	"errors"
	"reflect"
	"sync"

	"github.com/iancoleman/strcase"
)

// Norm base struct
type Norm struct {
	metas  map[reflect.Type]*modelMeta
	tables map[string]*modelMeta
	mut    sync.RWMutex
	config *Config
}

type Config struct {
	DefaultString string
}

var defaultConfig = &Config{DefaultString: "text"}

// NewNorm creates a new Norm instance
func NewNorm(config *Config) *Norm {
	if config == nil {
		config = defaultConfig
	}
	return &Norm{
		metas:  make(map[reflect.Type]*modelMeta),
		tables: make(map[string]*modelMeta),
		config: config,
	}
}

// AddModel registers a model with an explicit table name and returns a Model bound to obj.
func (n *Norm) AddModel(obj any, table string) *Model {
	meta := newModelMeta(n.config)

	err := meta.Parse(obj, table)
	if err != nil {
		panic(err)
	}

	n.mut.Lock()
	defer n.mut.Unlock()

	n.metas[meta.valType] = meta
	n.tables[table] = meta

	val := reflect.ValueOf(obj)
	return &Model{modelMeta: meta, val: val.Elem()}
}

// M returns a *Model bound to obj.
//
//	obj - must be a pointer to a struct.
//
// Metadata is cached by struct type. Each call returns a new Model bound to the given obj.
func (n *Norm) M(obj any) (*Model, error) {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		return nil, errors.New("obj must be pointer to struct")
	}

	n.mut.RLock()
	meta, ok := n.metas[val.Elem().Type()]
	n.mut.RUnlock()

	if !ok {
		return n.AddModel(obj, strcase.ToSnake(val.Elem().Type().Name())), nil
	}

	return &Model{modelMeta: meta, val: val.Elem()}, nil
}

func (n *Norm) Tables() []string {
	n.mut.RLock()
	defer n.mut.RUnlock()
	tables := make([]string, 0, len(n.tables))
	for table := range n.tables {
		tables = append(tables, table)
	}
	return tables
}
