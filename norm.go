package norm

import (
	"reflect"
	"sync"
)

// Norm base struct
type Norm struct {
	models map[reflect.Type]*Model
	mut    sync.Mutex
}

// NewNorm creates a new Norm instance
func NewNorm() *Norm {
	return &Norm{}
}

// AddModels adds several models to models cache
func (orm *Norm) AddModels(objs ...any) {
	for _, obj := range objs {
		orm.AddModel(obj)
	}
}

// AddModel adds model to models cache
func (orm *Norm) AddModel(obj any) *Model {
	model := NewModel()

	err := model.Parse(obj)
	if err != nil {
		panic(err)
	}

	if orm.models == nil {
		orm.models = make(map[reflect.Type]*Model)
	}

	orm.mut.Lock()
	defer orm.mut.Unlock()
	orm.models[model.valType] = model

	return model
}

// M returns *Model for object
//
// obj must be a pointer to a struct.
//
// If Model for the object was not found in the cache, then a new model is created and added to the cache.
func (orm *Norm) M(obj any) *Model {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("obj must be pointer to struct")
	}
	model, ok := orm.models[val.Elem().Type()]
	if !ok {
		model = orm.AddModel(obj)
	}

	model.currentObject = obj

	return model
}
