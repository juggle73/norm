package norm

import (
	"reflect"
	"sync"

	"github.com/iancoleman/strcase"
)

// Norm base struct
type Norm struct {
	models map[reflect.Type]*Model
	tables map[string]*Model
	mut    sync.RWMutex
	config *Config
}

type Config struct {
	DefaultString string
}

var defaultConfig = &Config{DefaultString: "text"}

// NewNorm creates a new Norm instance
func NewNorm(config *Config) *Norm {
	norm := &Norm{}
	if config == nil {
		norm.config = defaultConfig
	} else {
		norm.config = config
	}

	return norm
}

// AddModel adds model to models cache
func (n *Norm) AddModel(obj any, table string) *Model {
	model := NewModel(n.config)

	err := model.Parse(obj, table)
	if err != nil {
		panic(err)
	}

	n.mut.Lock()
	defer n.mut.Unlock()

	if n.models == nil {
		n.models = make(map[reflect.Type]*Model)
		n.tables = make(map[string]*Model)
	}
	n.models[model.valType] = model
	n.tables[table] = model

	return model
}

// M returns *Model for object
//
//	obj - must be a pointer to a struct.
//
// If Model for the object was not found in the cache, then a new model is created and added to the cache.
func (n *Norm) M(obj any) *Model {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("obj must be pointer to struct")
	}
	n.mut.RLock()
	model, ok := n.models[val.Elem().Type()]
	n.mut.RUnlock()
	if !ok {
		model = n.AddModel(obj, strcase.ToSnake(val.Elem().Type().Name()))
	}

	return model
}

// T trying to find registered model by table name and returns *Model for object
//
// If it was not found in the cache, then returns nil
func (n *Norm) T(table string) *Model {
	n.mut.RLock()
	defer n.mut.RUnlock()
	return n.tables[table]
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
