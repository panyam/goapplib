// Package storage is the root of goapplib's storage module. An app keeps its own records in its
// own store, and keeps the files those records point at in mounts. A mount is a folder or a
// GitHub repo, and later an S3 or GCS bucket. The app reaches a private mount through a
// connection, a sign-in the server holds.
//
// The module is separate from goapplib so that an app without mounts never downloads their
// dependencies. It shares goapplib's version and is tagged storage/vX.Y.Z on the same commit as
// vX.Y.Z. https://panyam.github.io/goapplib/reference/storage-module/ has the layout, the
// version policy and the dependency rule.
package storage
