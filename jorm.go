package jorm

import (
	"reflect"
	"sync"
)

type Jorm struct {
	models map[reflect.Type]*Model
	mut    sync.Mutex
}

func NewJorm() *Jorm {
	return &Jorm{}
}

func (orm *Jorm) AddModels(objs ...any) {
	for _, obj := range objs {
		orm.AddModel(obj)
	}
}

func (orm *Jorm) AddModel(obj any) *Model {
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

func (orm *Jorm) M(obj any) *Model {
	val := reflect.ValueOf(obj)
	if !isPointerToStruct(val) {
		panic("obj must be pointer to struct")
	}
	model, ok := orm.models[val.Elem().Type()]
	if !ok {
		return orm.AddModel(obj)
	}

	return model
}
