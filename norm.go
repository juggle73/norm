package norm

import (
	"reflect"
	"sync"
)

// Norm base struct
type Norm struct {
	models map[reflect.Type]*Model
	mut    sync.Mutex
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
func (norm *Norm) AddModel(obj any, table string) *Model {
	model := NewModel(norm.config)

	err := model.Parse(obj, table)
	if err != nil {
		panic(err)
	}

	if norm.models == nil {
		norm.models = make(map[reflect.Type]*Model)
	}

	norm.mut.Lock()
	defer norm.mut.Unlock()
	norm.models[model.valType] = model

	return model
}

// M returns *Model for object
//
// obj must be a pointer to a struct.
//
// If Model for the object was not found in the cache, then a new model is created and added to the cache.
func (norm *Norm) M(obj any) *Model {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("obj must be pointer to struct")
	}
	model, ok := norm.models[val.Elem().Type()]
	if !ok {
		model = norm.AddModel(obj, "")
	}

	model.currentObject = obj

	return model
}
