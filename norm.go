package norm

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"

	"github.com/iancoleman/strcase"
)

// Norm is the entry point for the norm library. It caches struct metadata
// so that reflection only happens once per type. Create one instance per
// application and reuse it — it is safe for concurrent use.
type Norm struct {
	metas  map[reflect.Type]*modelMeta
	tables map[string]*modelMeta
	mut    sync.RWMutex
	config *Config
}

// Config holds optional configuration for a [Norm] instance.
type Config struct {
	// DefaultString sets the PostgreSQL type used for Go string fields
	// when no dbType tag is specified. Defaults to "text".
	DefaultString string

	// DefaultTime sets the PostgreSQL type used for time.Time fields
	// when no dbType tag is specified. Defaults to "timestamptz".
	DefaultTime string

	// DefaultJSON sets the PostgreSQL type used for struct fields
	// serialized as JSON when no dbType tag is specified. Defaults to "jsonb".
	DefaultJSON string

	// JSONMarshal is the function used to marshal struct fields to JSON.
	// Defaults to [encoding/json.Marshal]. Replace with a faster
	// implementation (sonic, go-json, json-iterator) for better performance.
	//
	//	orm := norm.NewNorm(&norm.Config{
	//	    JSONMarshal: sonic.Marshal,
	//	})
	JSONMarshal func(v any) ([]byte, error)

	// JSONUnmarshal is the function used to unmarshal JSON into struct fields.
	// Defaults to [encoding/json.Unmarshal].
	JSONUnmarshal func(data []byte, v any) error
}

var defaultConfig = &Config{}

// NewNorm creates a new [Norm] instance. Pass nil for default configuration.
//
//	orm := norm.NewNorm(nil)
//	orm := norm.NewNorm(&norm.Config{DefaultString: "varchar"})
func NewNorm(config *Config) *Norm {
	if config == nil {
		config = defaultConfig
	}
	if config.DefaultString == "" {
		config.DefaultString = "text"
	}
	if config.DefaultTime == "" {
		config.DefaultTime = "timestamptz"
	}
	if config.DefaultJSON == "" {
		config.DefaultJSON = "jsonb"
	}
	if config.JSONMarshal == nil {
		config.JSONMarshal = json.Marshal
	}
	if config.JSONUnmarshal == nil {
		config.JSONUnmarshal = json.Unmarshal
	}
	return &Norm{
		metas:  make(map[reflect.Type]*modelMeta),
		tables: make(map[string]*modelMeta),
		config: config,
	}
}

// AddModel registers a struct with an explicit table name and returns
// a [Model] bound to obj. Panics if obj is not a struct or pointer to struct.
//
//	orm.AddModel(&User{}, "app_users")
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

// M returns a [Model] bound to obj. obj must be a pointer to a struct.
//
// Struct metadata is cached by type — reflection happens only on the first
// call for each struct type. Each call returns a new Model bound to the
// given obj instance.
//
//	user := User{Id: 1, Name: "John"}
//	m, err := orm.M(&user)
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

// Tables returns a list of all registered table names.
func (n *Norm) Tables() []string {
	n.mut.RLock()
	defer n.mut.RUnlock()
	tables := make([]string, 0, len(n.tables))
	for table := range n.tables {
		tables = append(tables, table)
	}
	return tables
}

// FieldsByTable returns field descriptors for a registered table.
// Returns nil if the table is not registered.
func (n *Norm) FieldsByTable(table string) []*Field {
	n.mut.RLock()
	defer n.mut.RUnlock()
	meta, ok := n.tables[table]
	if !ok {
		return nil
	}
	return meta.fields
}

// GetConfig returns the Norm configuration.
func (n *Norm) GetConfig() *Config {
	return n.config
}
