package jorm

import "reflect"

type Field struct {
	valType   reflect.Type
	name      string
	dbName    string
	tagValues []string
}
