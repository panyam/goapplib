package dsmigrate

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
)

// Get loads the entity at key into dst, a pointer to a struct, upgrading it to the current schema
// on the way. When it upgraded, it writes the result back if the stored entity hasn't changed
// since the read (see WriteBack). A failed write-back doesn't fail the read; it goes to
// reg.OnWriteBackError.
//
// It returns datastore.ErrNoSuchEntity for a missing entity, like client.Get.
func Get(ctx context.Context, client *datastore.Client, key *datastore.Key, dst any, reg *Registry) error {
	var props datastore.PropertyList
	if err := client.Get(ctx, key, &props); err != nil {
		return err
	}
	upgraded, changed, err := reg.Load(key.Kind, props, dst)
	if err != nil {
		return err
	}
	if changed {
		reg.writeBack(ctx, client, key, props, upgraded)
	}
	return nil
}

// GetAll runs q and loads the results into dst, a pointer to a slice of structs or of struct
// pointers, upgrading each one as Get does. q must not be keys-only. Each upgraded entity is
// written back on its own, under the same check as Get.
//
// An entity that hasn't been upgraded yet can't match a filter on a property its upgrader adds.
// Run Backfill before relying on such a query.
func GetAll(ctx context.Context, client *datastore.Client, q *datastore.Query, dst any, reg *Registry) ([]*datastore.Key, error) {
	slice := reflect.ValueOf(dst)
	if slice.Kind() != reflect.Pointer || slice.Elem().Kind() != reflect.Slice {
		return nil, fmt.Errorf("dsmigrate: GetAll needs a pointer to a slice, got %T", dst)
	}
	slice = slice.Elem()
	elemType := slice.Type().Elem()
	isPtr := elemType.Kind() == reflect.Pointer
	if isPtr {
		elemType = elemType.Elem()
	}

	var all []datastore.PropertyList
	keys, err := client.GetAll(ctx, q, &all)
	if err != nil {
		return nil, err
	}
	for i, props := range all {
		elem := reflect.New(elemType)
		upgraded, changed, err := reg.Load(keys[i].Kind, props, elem.Interface())
		if err != nil {
			return nil, err
		}
		if changed {
			reg.writeBack(ctx, client, keys[i], props, upgraded)
		}
		if isPtr {
			slice.Set(reflect.Append(slice, elem))
		} else {
			slice.Set(reflect.Append(slice, elem.Elem()))
		}
	}
	return keys, nil
}

// Put saves src, a pointer to a struct or a datastore.PropertyLoadSaver, at key with the kind's
// current version stamped on, so the entity is never treated as old. Inside a transaction, use
// reg.Save and tx.Put instead.
func Put(ctx context.Context, client *datastore.Client, key *datastore.Key, src any, reg *Registry) (*datastore.Key, error) {
	props, err := reg.Save(key.Kind, src)
	if err != nil {
		return nil, err
	}
	return client.Put(ctx, key, &props)
}

// WriteBack stores upgraded at key in a transaction, but only if the stored entity still equals
// original, the properties the upgrade started from. It reports written=false when the entity
// changed or vanished meanwhile: another instance upgraded it first, or something wrote a newer
// copy. That copy wins, and the next read upgrades it if it still needs it.
func WriteBack(ctx context.Context, client *datastore.Client, key *datastore.Key, original, upgraded datastore.PropertyList) (written bool, err error) {
	_, err = client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		written = false
		var current datastore.PropertyList
		if err := tx.Get(key, &current); err != nil {
			if errors.Is(err, datastore.ErrNoSuchEntity) {
				return nil
			}
			return err
		}
		if !sameProperties(current, original) {
			return nil
		}
		if _, err := tx.Put(key, &upgraded); err != nil {
			return err
		}
		written = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return written, nil
}

func (r *Registry) writeBack(ctx context.Context, client *datastore.Client, key *datastore.Key, original, upgraded datastore.PropertyList) {
	if _, err := WriteBack(ctx, client, key, original, upgraded); err != nil && r != nil && r.OnWriteBackError != nil {
		r.OnWriteBackError(key, err)
	}
}

// sameProperties compares two property lists regardless of the order of differently named
// properties. Values of one multi-valued property keep their order, which is meaningful.
func sameProperties(a, b datastore.PropertyList) bool {
	if len(a) != len(b) {
		return false
	}
	return reflect.DeepEqual(sortedByName(a), sortedByName(b))
}

func sortedByName(props datastore.PropertyList) datastore.PropertyList {
	out := append(datastore.PropertyList(nil), props...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// BackfillOptions tunes a Backfill run.
type BackfillOptions struct {
	// Namespace to scan. Empty is the default namespace.
	Namespace string
	// BatchSize is how many entities each query page reads. Zero means 100.
	BatchSize int
	// Limit stops the run after scanning this many entities, with a Cursor to resume from. Zero
	// scans the whole kind.
	Limit int
	// Cursor resumes a run from BackfillReport.Cursor. Empty starts at the beginning.
	Cursor string
	// DryRun counts the stale entities without writing anything.
	DryRun bool
}

// BackfillReport says what a Backfill run did.
type BackfillReport struct {
	Scanned  int // entities read
	Stale    int // entities below the kind's current version
	Upgraded int // stale entities written back (0 on a dry run)
	Skipped  int // stale entities that changed during the run and were left for the next pass
	// Cursor resumes the scan where this run stopped. It's empty when the run reached the end.
	Cursor string
}

// Backfill upgrades every stale entity of kind, page by page, while the app keeps serving. Each
// write goes through WriteBack, so it never overwrites a concurrent write, and a rerun finds
// nothing left to do. A run cut short by Limit, an error or a cancelled context reports a Cursor
// to resume from.
func Backfill(ctx context.Context, client *datastore.Client, kind string, reg *Registry, opts BackfillOptions) (BackfillReport, error) {
	var report BackfillReport
	schema, ok := reg.Schema(kind)
	if !ok {
		return report, fmt.Errorf("dsmigrate: no schema registered for kind %s", kind)
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 100
	}
	cursor := opts.Cursor
	for {
		q := datastore.NewQuery(kind).Namespace(opts.Namespace).Limit(batch)
		if cursor != "" {
			c, err := datastore.DecodeCursor(cursor)
			if err != nil {
				return report, fmt.Errorf("dsmigrate: bad cursor: %w", err)
			}
			q = q.Start(c)
		}
		it := client.Run(ctx, q)
		read := 0
		for {
			var props datastore.PropertyList
			key, err := it.Next(&props)
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				report.Cursor = cursor
				return report, err
			}
			read++
			report.Scanned++
			upgraded, changed, err := schema.Upgrade(props)
			if err != nil {
				report.Cursor = cursor
				return report, fmt.Errorf("dsmigrate: %v: %w", key, err)
			}
			if !changed {
				continue
			}
			report.Stale++
			if opts.DryRun {
				continue
			}
			written, err := WriteBack(ctx, client, key, props, upgraded)
			if err != nil {
				report.Cursor = cursor
				return report, fmt.Errorf("dsmigrate: writing back %v: %w", key, err)
			}
			if written {
				report.Upgraded++
			} else {
				report.Skipped++
			}
		}
		next, err := it.Cursor()
		if err != nil {
			return report, err
		}
		cursor = next.String()
		if read < batch {
			report.Cursor = ""
			return report, nil
		}
		if opts.Limit > 0 && report.Scanned >= opts.Limit {
			report.Cursor = cursor
			return report, nil
		}
	}
}
