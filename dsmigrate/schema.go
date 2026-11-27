package dsmigrate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"cloud.google.com/go/datastore"
)

// VersionProperty is the property that holds an entity's schema version. An entity without it is
// version 0, which is how entities written before a kind had a Schema look.
const VersionProperty = "schema_version"

// Upgrader turns the properties of an entity at one version into the next version's. It may add,
// rename, drop or rewrite properties, and must not change the key. It doesn't set
// VersionProperty; Schema.Upgrade does that after each step.
type Upgrader func(props datastore.PropertyList) (datastore.PropertyList, error)

// Schema describes one kind's versions. Upgrades[n] turns version n into n+1, so a Schema at
// Current 2 needs Upgrades[0] and Upgrades[1].
type Schema struct {
	Kind     string
	Current  int
	Upgrades map[int]Upgrader
}

// Validate reports a Schema that can't upgrade every version below Current.
func (s Schema) Validate() error {
	if s.Kind == "" {
		return errors.New("dsmigrate: schema has no kind")
	}
	if s.Current < 0 {
		return fmt.Errorf("dsmigrate: %s: current version %d is negative", s.Kind, s.Current)
	}
	for v := 0; v < s.Current; v++ {
		if s.Upgrades[v] == nil {
			return fmt.Errorf("dsmigrate: %s: no upgrader from version %d", s.Kind, v)
		}
	}
	return nil
}

// Version returns the entity's schema version: VersionProperty as an integer, or 0 when it's
// missing or not an integer.
func Version(props datastore.PropertyList) int {
	for _, p := range props {
		if p.Name == VersionProperty {
			if v, ok := p.Value.(int64); ok {
				return int(v)
			}
			return 0
		}
	}
	return 0
}

// setVersion returns props with VersionProperty set to v, replacing any existing one. The version
// isn't indexed: nothing queries by it, and Backfill scans the whole kind.
func setVersion(props datastore.PropertyList, v int) datastore.PropertyList {
	out := make(datastore.PropertyList, 0, len(props)+1)
	for _, p := range props {
		if p.Name != VersionProperty {
			out = append(out, p)
		}
	}
	return append(out, datastore.Property{Name: VersionProperty, Value: int64(v), NoIndex: true})
}

// withoutVersion returns props minus VersionProperty, for loading into a struct that has no field
// for it.
func withoutVersion(props datastore.PropertyList) datastore.PropertyList {
	out := make(datastore.PropertyList, 0, len(props))
	for _, p := range props {
		if p.Name != VersionProperty {
			out = append(out, p)
		}
	}
	return out
}

// Upgrade runs the upgraders from the entity's version up to Current and reports whether any ran.
// An entity at Current is returned as is. So is one above Current, which a newer build wrote: an
// older build can't know that schema, so it leaves the entity alone rather than rewrite it.
func (s Schema) Upgrade(props datastore.PropertyList) (out datastore.PropertyList, changed bool, err error) {
	v := Version(props)
	if v >= s.Current {
		return props, false, nil
	}
	out = props
	for ; v < s.Current; v++ {
		up := s.Upgrades[v]
		if up == nil {
			return nil, false, fmt.Errorf("dsmigrate: %s: no upgrader from version %d", s.Kind, v)
		}
		if out, err = up(out); err != nil {
			return nil, false, fmt.Errorf("dsmigrate: %s: upgrading from version %d: %w", s.Kind, v, err)
		}
		out = setVersion(out, v+1)
	}
	return out, true, nil
}

// Registry holds the Schema for each kind. A kind without one passes through unchanged: Get, GetAll
// and Put then behave like the plain client calls, apart from field mismatch handling.
type Registry struct {
	// IgnoreFieldMismatch drops a *datastore.ErrFieldMismatch from loads, the way a client built
	// with datastore.WithIgnoreFieldMismatch does. Set it when the stores using this registry run
	// on such a client, or legacy properties that no struct field takes will fail their reads.
	IgnoreFieldMismatch bool

	// OnWriteBackError, if set, hears about an upgrade a read couldn't write back. The read still
	// succeeds with the upgraded entity, and the next read or a Backfill tries the write again.
	OnWriteBackError func(key *datastore.Key, err error)

	mu      sync.RWMutex
	schemas map[string]Schema
}

// NewRegistry validates and registers the schemas. It fails on an invalid Schema or a kind
// registered twice.
func NewRegistry(schemas ...Schema) (*Registry, error) {
	r := &Registry{schemas: map[string]Schema{}}
	for _, s := range schemas {
		if err := r.Register(s); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register validates and adds a Schema. It fails on an invalid Schema or a kind registered twice.
func (r *Registry) Register(s Schema) error {
	if err := s.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.schemas == nil {
		r.schemas = map[string]Schema{}
	}
	if _, dup := r.schemas[s.Kind]; dup {
		return fmt.Errorf("dsmigrate: kind %s registered twice", s.Kind)
	}
	r.schemas[s.Kind] = s
	return nil
}

// Schema returns the kind's Schema, if one is registered. A nil Registry has none.
func (r *Registry) Schema(kind string) (Schema, bool) {
	if r == nil {
		return Schema{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[kind]
	return s, ok
}

// Upgrade runs the kind's Schema over props. A kind without one, or a nil Registry, returns props
// unchanged.
func (r *Registry) Upgrade(kind string, props datastore.PropertyList) (datastore.PropertyList, bool, error) {
	s, ok := r.Schema(kind)
	if !ok {
		return props, false, nil
	}
	return s.Upgrade(props)
}

// Load upgrades props, the properties stored at key, and loads them into dst, a pointer to a
// struct or a datastore.PropertyLoadSaver. The kind's Schema comes from key.Kind. It reports
// whether an upgrade ran, so the caller can write the result back. It doesn't write anything
// itself, which makes it the call to use inside a transaction.
//
// Load fills dst's key the way client.Get does. A struct field tagged datastore:"__key__" gets
// key, and a dst that implements datastore.KeyLoader gets LoadKey(key) instead. key must not be
// nil.
func (r *Registry) Load(key *datastore.Key, props datastore.PropertyList, dst any) (upgraded datastore.PropertyList, changed bool, err error) {
	if key == nil {
		return nil, false, errors.New("dsmigrate: Load needs the entity's key")
	}
	upgraded, changed, err = r.Upgrade(key.Kind, props)
	if err != nil {
		return nil, false, err
	}
	if err := r.loadStruct(dst, upgraded); err != nil {
		return nil, false, err
	}
	return upgraded, changed, loadKey(dst, key)
}

// Save turns src, a pointer to a struct or a datastore.PropertyLoadSaver, into properties with the
// kind's current version stamped on. A kind without a Schema gets no version.
func (r *Registry) Save(kind string, src any) (datastore.PropertyList, error) {
	props, err := saveProps(src)
	if err != nil {
		return nil, err
	}
	if s, ok := r.Schema(kind); ok {
		props = setVersion(props, s.Current)
	}
	return props, nil
}

func (r *Registry) loadStruct(dst any, props datastore.PropertyList) error {
	props = withoutVersion(props)
	var err error
	if pls, ok := dst.(datastore.PropertyLoadSaver); ok {
		err = pls.Load(props)
	} else {
		err = datastore.LoadStruct(dst, props)
	}
	var mismatch *datastore.ErrFieldMismatch
	if err != nil && r != nil && r.IgnoreFieldMismatch && errors.As(err, &mismatch) {
		return nil
	}
	return err
}

// loadKey gives dst its key the way client.Get does, since datastore.LoadStruct and a
// PropertyLoadSaver's Load never see it (#253).
func loadKey(dst any, key *datastore.Key) error {
	if kl, ok := dst.(datastore.KeyLoader); ok {
		return kl.LoadKey(key)
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	v = v.Elem()
	keyType := reflect.TypeOf(key)
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("datastore"), ",")
		if name == "__key__" && f.Type == keyType && f.IsExported() {
			v.Field(i).Set(reflect.ValueOf(key))
			return nil
		}
	}
	return nil
}

func saveProps(src any) (datastore.PropertyList, error) {
	if pls, ok := src.(datastore.PropertyLoadSaver); ok {
		props, err := pls.Save()
		return datastore.PropertyList(props), err
	}
	props, err := datastore.SaveStruct(src)
	return datastore.PropertyList(props), err
}
