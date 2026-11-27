package dsmigrate_test

import (
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/datastore"

	"github.com/panyam/goapplib/dsmigrate"
)

// The test schema follows an Identity through two changes. Version 0 is the legacy shape
// (identityType, identityKey, primaryUser). Version 1 renames them to type, value and user_id.
// Version 2 adds verified, false for every legacy identity.
func identitySchema() dsmigrate.Schema {
	return dsmigrate.Schema{
		Kind:    "Identity",
		Current: 2,
		Upgrades: map[int]dsmigrate.Upgrader{
			0: func(in datastore.PropertyList) (datastore.PropertyList, error) {
				rename := map[string]string{"identityType": "type", "identityKey": "value", "primaryUser": "user_id"}
				var out datastore.PropertyList
				for _, p := range in {
					if to, ok := rename[p.Name]; ok {
						p.Name = to
					}
					out = append(out, p)
				}
				return out, nil
			},
			1: func(in datastore.PropertyList) (datastore.PropertyList, error) {
				return append(in, datastore.Property{Name: "verified", Value: false}), nil
			},
		},
	}
}

type identity struct {
	Type     string `datastore:"type"`
	Value    string `datastore:"value"`
	UserID   string `datastore:"user_id"`
	Verified bool   `datastore:"verified"`
}

func legacy(user string) datastore.PropertyList {
	return datastore.PropertyList{
		{Name: "identityType", Value: "email"},
		{Name: "identityKey", Value: user + "@example.com"},
		{Name: "primaryUser", Value: user},
	}
}

func prop(props datastore.PropertyList, name string) (any, bool) {
	for _, p := range props {
		if p.Name == name {
			return p.Value, true
		}
	}
	return nil, false
}

func TestUpgrade_RunsEveryStepFromVersionZero(t *testing.T) {
	out, changed, err := identitySchema().Upgrade(legacy("alice"))
	if err != nil || !changed {
		t.Fatalf("Upgrade: changed=%v err=%v", changed, err)
	}
	if got := dsmigrate.Version(out); got != 2 {
		t.Errorf("version = %d, want 2", got)
	}
	for name, want := range map[string]any{"type": "email", "user_id": "alice", "verified": false} {
		if got, _ := prop(out, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if _, ok := prop(out, "primaryUser"); ok {
		t.Errorf("primaryUser survived the upgrade")
	}
}

func TestUpgrade_StartsFromTheStoredVersion(t *testing.T) {
	v1 := datastore.PropertyList{
		{Name: "type", Value: "email"},
		{Name: dsmigrate.VersionProperty, Value: int64(1)},
	}
	out, changed, err := identitySchema().Upgrade(v1)
	if err != nil || !changed {
		t.Fatalf("Upgrade: changed=%v err=%v", changed, err)
	}
	if got, _ := prop(out, "verified"); got != false {
		t.Errorf("verified = %v, want false", got)
	}
	if n := strings.Count(propNames(out), dsmigrate.VersionProperty); n != 1 {
		t.Errorf("%d version properties, want 1", n)
	}
}

func TestUpgrade_LeavesCurrentAndNewerAlone(t *testing.T) {
	for _, v := range []int64{2, 3} {
		in := datastore.PropertyList{{Name: "type", Value: "email"}, {Name: dsmigrate.VersionProperty, Value: v}}
		out, changed, err := identitySchema().Upgrade(in)
		if err != nil || changed || len(out) != len(in) {
			t.Errorf("version %d: changed=%v err=%v props=%v", v, changed, err, out)
		}
	}
}

func TestUpgrade_ReportsAFailingUpgrader(t *testing.T) {
	s := identitySchema()
	boom := errors.New("boom")
	s.Upgrades[1] = func(datastore.PropertyList) (datastore.PropertyList, error) { return nil, boom }
	if _, _, err := s.Upgrade(legacy("alice")); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}

func TestValidate(t *testing.T) {
	missing := identitySchema()
	delete(missing.Upgrades, 1)
	for name, s := range map[string]dsmigrate.Schema{
		"no kind":          {Current: 0},
		"missing upgrader": missing,
		"negative":         {Kind: "K", Current: -1},
	} {
		if s.Validate() == nil {
			t.Errorf("%s: Validate passed", name)
		}
	}
	if err := identitySchema().Validate(); err != nil {
		t.Errorf("valid schema: %v", err)
	}
}

func TestRegistry_RejectsADuplicateKind(t *testing.T) {
	if _, err := dsmigrate.NewRegistry(identitySchema(), identitySchema()); err == nil {
		t.Error("duplicate kind accepted")
	}
}

func TestRegistry_SaveStampsTheCurrentVersion(t *testing.T) {
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	props, err := reg.Save("Identity", &identity{Type: "email", Value: "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got := dsmigrate.Version(props); got != 2 {
		t.Errorf("version = %d, want 2", got)
	}
	other, _ := reg.Save("Unregistered", &identity{})
	if _, ok := prop(other, dsmigrate.VersionProperty); ok {
		t.Error("an unregistered kind got a version")
	}
}

func TestRegistry_LoadUpgradesAndHidesTheVersion(t *testing.T) {
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	var got identity
	_, changed, err := reg.Load(datastore.NameKey("Identity", "alice", nil), legacy("alice"), &got)
	if err != nil || !changed {
		t.Fatalf("Load: changed=%v err=%v", changed, err)
	}
	if got.UserID != "alice" || got.Value != "alice@example.com" {
		t.Errorf("loaded %+v", got)
	}
}

func TestRegistry_FieldMismatch(t *testing.T) {
	extra := append(legacy("alice"), datastore.Property{Name: "isActive", Value: true})
	strict, _ := dsmigrate.NewRegistry(identitySchema())
	var mismatch *datastore.ErrFieldMismatch
	if _, _, err := strict.Load(datastore.NameKey("Identity", "alice", nil), extra, &identity{}); !errors.As(err, &mismatch) {
		t.Errorf("strict registry: err = %v, want ErrFieldMismatch", err)
	}
	lenient, _ := dsmigrate.NewRegistry(identitySchema())
	lenient.IgnoreFieldMismatch = true
	var got identity
	if _, _, err := lenient.Load(datastore.NameKey("Identity", "alice", nil), extra, &got); err != nil || got.UserID != "alice" {
		t.Errorf("lenient registry: got %+v err %v", got, err)
	}
}

func propNames(props datastore.PropertyList) string {
	var b strings.Builder
	for _, p := range props {
		b.WriteString(p.Name + " ")
	}
	return b.String()
}

type keyedIdentity struct {
	Key      *datastore.Key `datastore:"__key__"`
	UserID   string         `datastore:"user_id"`
	Value    string         `datastore:"value"`
	Type     string         `datastore:"type"`
	Verified bool           `datastore:"verified"`
}

// keyLoaderIdentity takes its key through datastore.KeyLoader rather than a tagged field.
type keyLoaderIdentity struct {
	identity
	key *datastore.Key
}

func (k *keyLoaderIdentity) Load(props []datastore.Property) error {
	return datastore.LoadStruct(&k.identity, props)
}

func (k *keyLoaderIdentity) Save() ([]datastore.Property, error) {
	return datastore.SaveStruct(&k.identity)
}

func (k *keyLoaderIdentity) LoadKey(key *datastore.Key) error {
	k.key = key
	return nil
}

func TestRegistry_LoadFillsTheKeyField(t *testing.T) {
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	k := datastore.NameKey("Identity", "email:alice", nil)
	var got keyedIdentity
	if _, _, err := reg.Load(k, legacy("alice"), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Key.Equal(k) || got.UserID != "alice" {
		t.Errorf("loaded %+v, want key %v", got, k)
	}
}

func TestRegistry_LoadCallsKeyLoader(t *testing.T) {
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	k := datastore.NameKey("Identity", "email:alice", nil)
	var got keyLoaderIdentity
	if _, _, err := reg.Load(k, legacy("alice"), &got); err != nil {
		t.Fatal(err)
	}
	if !got.key.Equal(k) || got.UserID != "alice" {
		t.Errorf("loaded %+v, want key %v", got, k)
	}
}

func TestRegistry_LoadNeedsAKey(t *testing.T) {
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	if _, _, err := reg.Load(nil, legacy("alice"), &identity{}); err == nil {
		t.Error("Load with a nil key succeeded")
	}
}
