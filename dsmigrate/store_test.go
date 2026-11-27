package dsmigrate_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/panyam/goapplib/dsmigrate"
)

// These run against Datastore, or the emulator when DATASTORE_EMULATOR_HOST is set, and skip
// unless DATASTORE_PROJECT_ID is set. make test-datastore starts the emulator and sets both.
// Each test uses its own namespace.

func setup(t *testing.T) (context.Context, *datastore.Client, string) {
	t.Helper()
	project := os.Getenv("DATASTORE_PROJECT_ID")
	if project == "" {
		t.Skip("DATASTORE_PROJECT_ID not set; make test-datastore runs these against the emulator")
	}
	ctx := context.Background()
	c, err := datastore.NewClient(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return ctx, c, fmt.Sprintf("dsmigrate-%s-%d", t.Name(), time.Now().UnixNano())
}

func key(ns, name string) *datastore.Key {
	k := datastore.NameKey("Identity", name, nil)
	k.Namespace = ns
	return k
}

func raw(t *testing.T, ctx context.Context, c *datastore.Client, k *datastore.Key) datastore.PropertyList {
	t.Helper()
	var props datastore.PropertyList
	if err := c.Get(ctx, k, &props); err != nil {
		t.Fatalf("raw get %v: %v", k, err)
	}
	return props
}

func putRaw(t *testing.T, ctx context.Context, c *datastore.Client, k *datastore.Key, props datastore.PropertyList) {
	t.Helper()
	if _, err := c.Put(ctx, k, &props); err != nil {
		t.Fatal(err)
	}
}

func TestGet_UpgradesALegacyEntityAndWritesItBack(t *testing.T) {
	ctx, c, ns := setup(t)
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	k := key(ns, "email:alice@example.com")
	putRaw(t, ctx, c, k, legacy("alice"))

	var got identity
	if err := dsmigrate.Get(ctx, c, k, &got, reg); err != nil {
		t.Fatal(err)
	}
	if got.UserID != "alice" || got.Type != "email" {
		t.Errorf("loaded %+v", got)
	}
	stored := raw(t, ctx, c, k)
	if v := dsmigrate.Version(stored); v != 2 {
		t.Errorf("stored version = %d, want 2", v)
	}
	if _, ok := prop(stored, "primaryUser"); ok {
		t.Error("stored entity still has primaryUser")
	}
}

func TestGet_LeavesACurrentEntityUntouched(t *testing.T) {
	ctx, c, ns := setup(t)
	var calls atomic.Int32
	s := identitySchema()
	for v, up := range s.Upgrades {
		up := up
		s.Upgrades[v] = func(p datastore.PropertyList) (datastore.PropertyList, error) { calls.Add(1); return up(p) }
	}
	reg, _ := dsmigrate.NewRegistry(s)
	k := key(ns, "email:bob@example.com")
	if _, err := dsmigrate.Put(ctx, c, k, &identity{Type: "email", Value: "bob@example.com", UserID: "bob"}, reg); err != nil {
		t.Fatal(err)
	}
	var got identity
	if err := dsmigrate.Get(ctx, c, k, &got, reg); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || got.UserID != "bob" {
		t.Errorf("upgraders ran %d times, loaded %+v", calls.Load(), got)
	}
}

func TestGet_MissingEntity(t *testing.T) {
	ctx, c, ns := setup(t)
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	if err := dsmigrate.Get(ctx, c, key(ns, "nobody"), &identity{}, reg); err != datastore.ErrNoSuchEntity {
		t.Errorf("err = %v, want ErrNoSuchEntity", err)
	}
}

// An old build still serving writes the entity between our read and our write-back. Its write
// wins, and the write-back reports that it didn't write.
func TestWriteBack_KeepsAConcurrentWrite(t *testing.T) {
	ctx, c, ns := setup(t)
	k := key(ns, "email:carol@example.com")
	original := legacy("carol")
	putRaw(t, ctx, c, k, original)
	upgraded, _, _ := identitySchema().Upgrade(original)

	concurrent := legacy("carol-renamed")
	putRaw(t, ctx, c, k, concurrent)

	written, err := dsmigrate.WriteBack(ctx, c, k, original, upgraded)
	if err != nil || written {
		t.Fatalf("WriteBack: written=%v err=%v", written, err)
	}
	if got, _ := prop(raw(t, ctx, c, k), "primaryUser"); got != "carol-renamed" {
		t.Errorf("primaryUser = %v, want the concurrent write's carol-renamed", got)
	}
}

func TestGet_ConcurrentReadersAgree(t *testing.T) {
	ctx, c, ns := setup(t)
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	var failed atomic.Int32
	reg.OnWriteBackError = func(*datastore.Key, error) { failed.Add(1) }
	k := key(ns, "email:dave@example.com")
	putRaw(t, ctx, c, k, legacy("dave"))

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var got identity
			if err := dsmigrate.Get(ctx, c, k, &got, reg); err != nil {
				errs <- err
				return
			}
			if got.UserID != "dave" {
				errs <- fmt.Errorf("loaded %+v", got)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if v := dsmigrate.Version(raw(t, ctx, c, k)); v != 2 {
		t.Errorf("stored version = %d, want 2", v)
	}
	// Losing a write-back race is expected and harmless; it only matters that every read worked.
	t.Logf("write-backs that lost a race: %d", failed.Load())
}

func TestGetAll_UpgradesEachResult(t *testing.T) {
	ctx, c, ns := setup(t)
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	for _, u := range []string{"erin", "frank", "gail"} {
		putRaw(t, ctx, c, key(ns, "email:"+u), legacy(u))
	}
	var got []*identity
	keys, err := dsmigrate.GetAll(ctx, c, datastore.NewQuery("Identity").Namespace(ns), &got, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || len(keys) != 3 {
		t.Fatalf("got %d results, %d keys", len(got), len(keys))
	}
	for i, g := range got {
		if g.UserID == "" || dsmigrate.Version(raw(t, ctx, c, keys[i])) != 2 {
			t.Errorf("result %d: %+v not upgraded", i, g)
		}
	}
	var values []identity
	if _, err := dsmigrate.GetAll(ctx, c, datastore.NewQuery("Identity").Namespace(ns), &values, reg); err != nil || len(values) != 3 {
		t.Errorf("slice of values: %d results, err %v", len(values), err)
	}
}

func TestBackfill_ResumesAndFinishes(t *testing.T) {
	ctx, c, ns := setup(t)
	reg, _ := dsmigrate.NewRegistry(identitySchema())
	for i := 0; i < 25; i++ {
		putRaw(t, ctx, c, key(ns, fmt.Sprintf("legacy-%02d", i)), legacy(fmt.Sprintf("u%02d", i)))
	}
	for i := 0; i < 5; i++ {
		if _, err := dsmigrate.Put(ctx, c, key(ns, fmt.Sprintf("current-%d", i)), &identity{UserID: "x"}, reg); err != nil {
			t.Fatal(err)
		}
	}

	dry, err := dsmigrate.Backfill(ctx, c, "Identity", reg, dsmigrate.BackfillOptions{Namespace: ns, DryRun: true})
	if err != nil || dry.Scanned != 30 || dry.Stale != 25 || dry.Upgraded != 0 {
		t.Fatalf("dry run: %+v err %v", dry, err)
	}
	if v := dsmigrate.Version(raw(t, ctx, c, key(ns, "legacy-00"))); v != 0 {
		t.Fatalf("the dry run wrote: version %d", v)
	}

	first, err := dsmigrate.Backfill(ctx, c, "Identity", reg, dsmigrate.BackfillOptions{Namespace: ns, BatchSize: 10, Limit: 10})
	if err != nil || first.Scanned != 10 || first.Cursor == "" {
		t.Fatalf("first page: %+v err %v", first, err)
	}
	rest, err := dsmigrate.Backfill(ctx, c, "Identity", reg, dsmigrate.BackfillOptions{Namespace: ns, BatchSize: 10, Cursor: first.Cursor})
	if err != nil || rest.Cursor != "" {
		t.Fatalf("rest: %+v err %v", rest, err)
	}
	if total := first.Upgraded + rest.Upgraded; total != 25 {
		t.Errorf("upgraded %d, want 25 (first %+v, rest %+v)", total, first, rest)
	}

	again, err := dsmigrate.Backfill(ctx, c, "Identity", reg, dsmigrate.BackfillOptions{Namespace: ns})
	if err != nil || again.Stale != 0 || again.Scanned != 30 {
		t.Errorf("rerun: %+v err %v", again, err)
	}
}

func TestBackfill_UnregisteredKind(t *testing.T) {
	ctx, c, _ := setup(t)
	reg, _ := dsmigrate.NewRegistry()
	if _, err := dsmigrate.Backfill(ctx, c, "Identity", reg, dsmigrate.BackfillOptions{}); err == nil {
		t.Error("Backfill ran without a schema")
	}
}
