// Package mounts names the places an app's files live. A mount is a folder or a GitHub repo, and
// later an S3 or GCS bucket. Each is reached by a name, and a file in it by a mount:// URI. An app
// keeps its own records in its own store, and those records point into mounts by URI.
//
// This package holds the contract: the Mount record, the URI and name rules, the interfaces a
// driver and a record store implement, and the errors they return. The design, with how agni and
// notation use it, is at https://panyam.github.io/goapplib/reference/mounts/.
package mounts

import "time"

// Kind says which driver serves a mount.
type Kind string

const (
	// KindFolder is a folder on the server's disk. Folder mounts come from the server's own
	// configuration, never from a request, since a client may not pick a server folder.
	KindFolder Kind = "folder"

	// KindGitHub is a GitHub repository, read through GitHub's REST API.
	KindGitHub Kind = "github"
)

// Mode says whether a mount's files can change after it is made.
type Mode string

const (
	// ModeSnapshot pins the mount at the version its ref resolved to when it was made. Its files
	// never change, so a record that points into it keeps meaning the same bytes. agni uses this
	// for designs.
	ModeSnapshot Mode = "snapshot"

	// ModeLive reads the source's latest version on each open, and a Writer can change it.
	// notation uses this for notations it edits.
	ModeLive Mode = "live"
)

// Mount is the library's record of one mount. An app keeps any fields of its own in its own
// records, keyed by Name.
type Mount struct {
	// Name is the mount's handle in URIs: mount://<Name>/<path>. ValidName gives the rule.
	Name string

	Kind Kind

	// Source says where the files come from, in the driver's own form: "github://owner/repo"
	// for GitHub, or an absolute path for a folder.
	Source string

	// Ref is what the mount was asked for, such as a branch, a tag or a commit. Empty means the
	// source's default.
	Ref string

	// Version is what Ref resolved to when the mount was made, such as a commit sha. The driver
	// defines its format. For a snapshot mount the files are those at Version. A live mount has
	// moved on since, and its Writer returns each new version.
	Version string

	Mode Mode

	// Connection is the ID of the connection the driver reads through (package connections).
	// Empty means the source is public, or local.
	Connection string

	// ConnectionOwner is the subject of the person whose connection Connection is. Empty means a
	// server connection. Only that person can open a mount over their connection, since the
	// connections service gives a person's token to them alone.
	ConnectionOwner string

	// Manifest is the hash of the blob listing a snapshot's files and their hashes, which is
	// what rebuilds a snapshot's view after a restart. It is empty for a live mount.
	Manifest string

	Created time.Time
}
