// Package dsmigrate upgrades Datastore entities to the current schema while the app keeps serving,
// instead of in one offline rewrite.
//
// Each kind gets a Schema: its current version and one Upgrader per older version. An entity
// carries its version in the schema_version property, and an entity without one is version 0.
//
// Picture a user who last logged in under an old build. Their Identity entity is version 0. The
// new build reads it through Get, which runs the upgraders from 0 up to the current version and
// loads the result into the caller's struct. Get then writes the upgraded entity back, but only if
// the stored entity is still exactly what it read, so a concurrent write from another instance,
// or from an old build still serving, is never overwritten. Put stamps the current version on
// every write, so new entities never look old.
//
// Reads alone never reach an entity that nobody reads, and a query that filters on a field an
// upgrader adds can't find an entity that hasn't been upgraded yet. Backfill walks a whole kind
// and upgrades what it finds, in batches, while the app serves. It resumes from a cursor, and
// rerunning it changes nothing.
//
// The module depends only on cloud.google.com/go/datastore, so libraries that keep entities in
// Datastore (oneauth's stores, goapplib's content services) can use it without pulling in goapplib.
// It shares goapplib's version and is tagged dsmigrate/vX.Y.Z on the same commit as vX.Y.Z.
package dsmigrate
