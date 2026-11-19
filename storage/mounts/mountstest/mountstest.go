// Package mountstest checks that a mounts.RecordStore keeps the contract the registry relies on.
// A backend's test calls RunRecordStore with a factory for a fresh, empty store, the way oneauth's
// usertokentest checks user-token stores.
package mountstest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
)

// Factory returns a new, empty store. Calling it twice with the same t returns two stores over
// separate data, unless the backend's test means otherwise.
type Factory func(t *testing.T) mounts.RecordStore

// SharedFactory returns two stores over the same data, as two processes sharing one folder or one
// database would see it. RunShared uses it to check that each sees the other's changes.
type SharedFactory func(t *testing.T) (a, b mounts.RecordStore)

// Sample returns a record with every field set, so a round trip checks that none is dropped.
func Sample(name string) mounts.Mount {
	return mounts.Mount{
		Name:       name,
		Kind:       mounts.KindGitHub,
		Source:     "github://acme/" + name,
		Ref:        "main",
		Version:    "0123456789abcdef0123456789abcdef01234567",
		Mode:       mounts.ModeSnapshot,
		Connection: "acme-app",
		Manifest:   "sha256:" + fmt.Sprintf("%064x", len(name)),
		Created:    time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC),
	}
}

// RunRecordStore runs every contract check against stores from newStore.
func RunRecordStore(t *testing.T, newStore Factory) {
	ctx := context.Background()

	t.Run("EmptyListsNothing", func(t *testing.T) {
		got, err := newStore(t).ListMounts(ctx)
		if err != nil || len(got) != 0 {
			t.Fatalf("ListMounts = %v, %v; want nothing", got, err)
		}
	})

	t.Run("GetMissingIsNotFound", func(t *testing.T) {
		if _, err := newStore(t).GetMount(ctx, "nope"); !errors.Is(err, mounts.ErrNotFound) {
			t.Fatalf("GetMount = %v, want ErrNotFound", err)
		}
	})

	t.Run("CreateThenGetRoundTripsEveryField", func(t *testing.T) {
		s := newStore(t)
		want := Sample("jetson")
		if err := s.CreateMount(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetMount(ctx, "jetson")
		if err != nil {
			t.Fatal(err)
		}
		if !got.Created.Equal(want.Created) {
			t.Errorf("Created = %v, want %v", got.Created, want.Created)
		}
		got.Created = want.Created
		if got != want {
			t.Errorf("GetMount = %+v\nwant %+v", got, want)
		}
	})

	t.Run("CreateExistingIsExists", func(t *testing.T) {
		s := newStore(t)
		if err := s.CreateMount(ctx, Sample("a")); err != nil {
			t.Fatal(err)
		}
		other := Sample("a")
		other.Source = "github://acme/other"
		if err := s.CreateMount(ctx, other); !errors.Is(err, mounts.ErrExists) {
			t.Fatalf("second CreateMount = %v, want ErrExists", err)
		}
		if got, _ := s.GetMount(ctx, "a"); got.Source != Sample("a").Source {
			t.Errorf("the losing create replaced the record: source %q", got.Source)
		}
	})

	t.Run("CreateRefusesABadName", func(t *testing.T) {
		if err := newStore(t).CreateMount(ctx, Sample("../x")); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Fatalf("CreateMount = %v, want ErrInvalidArgument", err)
		}
	})

	t.Run("ListIsInNameOrder", func(t *testing.T) {
		s := newStore(t)
		for _, n := range []string{"c", "a", "b"} {
			if err := s.CreateMount(ctx, Sample(n)); err != nil {
				t.Fatal(err)
			}
		}
		got, err := s.ListMounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, m := range got {
			names = append(names, m.Name)
		}
		if fmt.Sprint(names) != "[a b c]" {
			t.Errorf("ListMounts names = %v, want [a b c]", names)
		}
	})

	t.Run("DeleteRemovesAndIsIdempotent", func(t *testing.T) {
		s := newStore(t)
		if err := s.CreateMount(ctx, Sample("a")); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteMount(ctx, "a"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetMount(ctx, "a"); !errors.Is(err, mounts.ErrNotFound) {
			t.Errorf("GetMount after delete = %v, want ErrNotFound", err)
		}
		if err := s.DeleteMount(ctx, "a"); err != nil {
			t.Errorf("deleting a missing record = %v, want nil", err)
		}
		if err := s.CreateMount(ctx, Sample("a")); err != nil {
			t.Errorf("creating again after delete = %v", err)
		}
	})

	t.Run("GenerationChangesOnCreateAndDeleteOnly", func(t *testing.T) {
		s := newStore(t)
		g0 := generation(t, s)
		if g := generation(t, s); g != g0 {
			t.Fatalf("Generation changed with no write: %q then %q", g0, g)
		}
		if err := s.CreateMount(ctx, Sample("a")); err != nil {
			t.Fatal(err)
		}
		g1 := generation(t, s)
		if g1 == g0 {
			t.Errorf("Generation didn't change on create: %q", g1)
		}
		if err := s.DeleteMount(ctx, "a"); err != nil {
			t.Fatal(err)
		}
		if g2 := generation(t, s); g2 == g1 {
			t.Errorf("Generation didn't change on delete: %q", g2)
		}
	})

	t.Run("ConcurrentCreatesOneWins", func(t *testing.T) {
		s := newStore(t)
		const n = 16
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m := Sample("race")
				m.Source = fmt.Sprintf("github://acme/r%d", i)
				errs[i] = s.CreateMount(ctx, m)
			}()
		}
		wg.Wait()
		won := 0
		for _, err := range errs {
			switch {
			case err == nil:
				won++
			case !errors.Is(err, mounts.ErrExists):
				t.Errorf("a losing create returned %v, want ErrExists", err)
			}
		}
		if won != 1 {
			t.Errorf("%d creates won, want 1", won)
		}
	})
}

// RunShared checks that two stores over the same data see each other's creates and deletes, and
// that a create race between them has one winner.
func RunShared(t *testing.T, newShared SharedFactory) {
	ctx := context.Background()

	t.Run("EachSeesTheOthersWrites", func(t *testing.T) {
		a, b := newShared(t)
		gb := generation(t, b)
		if err := a.CreateMount(ctx, Sample("x")); err != nil {
			t.Fatal(err)
		}
		if g := generation(t, b); g == gb {
			t.Errorf("b's Generation didn't change after a's create")
		}
		if _, err := b.GetMount(ctx, "x"); err != nil {
			t.Errorf("b.GetMount after a's create = %v", err)
		}
		if err := b.DeleteMount(ctx, "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := a.GetMount(ctx, "x"); !errors.Is(err, mounts.ErrNotFound) {
			t.Errorf("a.GetMount after b's delete = %v, want ErrNotFound", err)
		}
	})

	t.Run("CreateRaceAcrossStoresOneWins", func(t *testing.T) {
		a, b := newShared(t)
		errA := a.CreateMount(ctx, Sample("y"))
		errB := b.CreateMount(ctx, Sample("y"))
		if errA != nil || !errors.Is(errB, mounts.ErrExists) {
			t.Errorf("creates = %v, %v; want nil, ErrExists", errA, errB)
		}
	})
}

func generation(t *testing.T, s mounts.RecordStore) string {
	t.Helper()
	g, err := s.Generation(context.Background())
	if err != nil {
		t.Fatalf("Generation: %v", err)
	}
	return g
}
