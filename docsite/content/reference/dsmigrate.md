---
title: "Online schema migrations (dsmigrate)"
description: "Upgrade Datastore entities to a new schema while the app keeps serving: upgraders per version, upgrade on read with a guarded write-back, and a resumable backfill."
---

Picture an app with a few hundred users whose Identity entities still have the shape an older release wrote. A one-off migration means a maintenance window: stop the app, rewrite every entity, deploy the build that reads the new shape. Do that on every schema change and each upgrade turns into an outage.

`github.com/panyam/goapplib/dsmigrate` makes the upgrade happen while the app serves. It's a Go module of its own in goapplib's repo, versioned with goapplib and tagged `dsmigrate/vX.Y.Z`. Its only dependency is `cloud.google.com/go/datastore`, so libraries that keep entities in Datastore can use it without pulling in goapplib.

## Versions and upgraders

Each kind gets a `Schema`: its current version, and one `Upgrader` per older version. `Upgrades[n]` turns version `n` into `n+1`. An entity carries its version in the `schema_version` property. An entity without one is version 0, which is how everything written before the kind had a schema looks.

```go
reg, err := dsmigrate.NewRegistry(dsmigrate.Schema{
    Kind:    "Identity",
    Current: 1,
    Upgrades: map[int]dsmigrate.Upgrader{
        0: renameLegacyIdentityFields, // func(datastore.PropertyList) (datastore.PropertyList, error)
    },
})
```

Upgraders work on raw properties, so they can read fields the current struct no longer has.

## Reads upgrade, writes stamp

`dsmigrate.Get` and `dsmigrate.GetAll` load the raw properties, run the upgraders from the entity's version up to `Current`, and load the result into the caller's struct. When an upgrade ran, they write it back in a transaction, but only if the stored entity still equals what they read. If another instance upgraded it first, or an old build still serving wrote it meanwhile, that write wins, and the next read upgrades it if it still needs it. A failed write-back never fails the read; `Registry.OnWriteBackError` hears about it.

`dsmigrate.Put` stamps the current version on every write, so a new entity never looks old. Inside a transaction, use `reg.Load` to upgrade without writing, and `reg.Save` to get stamped properties for `tx.Put`.

An entity written by a newer build, above `Current`, is left alone. An older build can't know that schema, so it doesn't rewrite it.

## Backfill

Reads never reach entities nobody reads. And a query that filters on a property an upgrader adds can't match an entity that hasn't been upgraded yet. `dsmigrate.Backfill` walks a kind page by page and upgrades what it finds, through the same guarded write-back:

```go
report, err := dsmigrate.Backfill(ctx, client, "Identity", reg, dsmigrate.BackfillOptions{DryRun: true})
// report.Scanned, report.Stale, report.Upgraded, report.Skipped
```

A run cut short by `Limit`, an error or a cancelled context returns a `Cursor` to resume from, and a rerun finds nothing left to do.

## Rolling back

Once an entity is upgraded, a build older than the schema can't read the new fields. So a rollback past a schema change needs the old entities back, from an export taken before the deploy. An upgrader that keeps writing the old fields beside the new ones avoids that, at the cost of dropping them in a later version.
