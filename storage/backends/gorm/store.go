// Package gormstore keeps mount records and connection records in a database through GORM, so
// several servers can share them without a shared folder. It works with any database GORM
// supports, such as PostgreSQL, MySQL or SQLite.
//
// It is a Go module of its own, github.com/panyam/goapplib/storage/backends/gorm, so an app that
// keeps its records in a folder never downloads GORM.
package gormstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/mounts"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// mountRow is a mount record's table row. Its column names are the schema, so renaming a Go field
// doesn't change the table.
type mountRow struct {
	Name       string    `gorm:"column:name;primaryKey;size:64"`
	Kind       string    `gorm:"column:kind;size:32"`
	Source     string    `gorm:"column:source"`
	Ref        string    `gorm:"column:ref"`
	Version    string    `gorm:"column:version"`
	Mode       string    `gorm:"column:mode;size:16"`
	Connection string    `gorm:"column:connection;size:64"`
	ConnOwner  string    `gorm:"column:connection_owner;size:256"`
	Manifest   string    `gorm:"column:manifest;size:80"`
	Created    time.Time `gorm:"column:created"`
}

func (mountRow) TableName() string { return "goapplib_mounts" }

// generationRow is the one row whose counter each create and delete bumps.
type generationRow struct {
	ID      int   `gorm:"column:id;primaryKey;autoIncrement:false"`
	Counter int64 `gorm:"column:counter"`
}

func (generationRow) TableName() string { return "goapplib_mount_generation" }

func toMountRow(m mounts.Mount) mountRow {
	return mountRow{m.Name, string(m.Kind), m.Source, m.Ref, m.Version, string(m.Mode), m.Connection, m.ConnectionOwner, m.Manifest, m.Created.UTC()}
}

func (r mountRow) mount() mounts.Mount {
	return mounts.Mount{Name: r.Name, Kind: mounts.Kind(r.Kind), Source: r.Source, Ref: r.Ref, Version: r.Version,
		Mode: mounts.Mode(r.Mode), Connection: r.Connection, ConnectionOwner: r.ConnOwner, Manifest: r.Manifest, Created: r.Created.UTC()}
}

// MountStore is a mounts.RecordStore over a GORM database. Several servers can share one
// database: a create is exclusive across them, and each sees the others' changes through
// Generation.
type MountStore struct{ db *gorm.DB }

// NewMountStore migrates the goapplib_mounts and goapplib_mount_generation tables in db and
// returns a store over them.
func NewMountStore(db *gorm.DB) (*MountStore, error) {
	if err := db.AutoMigrate(&mountRow{}, &generationRow{}); err != nil {
		return nil, err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&generationRow{ID: 1}).Error; err != nil {
		return nil, err
	}
	return &MountStore{db: db}, nil
}

// ListMounts returns every record, in name order.
func (s *MountStore) ListMounts(ctx context.Context) ([]mounts.Mount, error) {
	var rows []mountRow
	if err := s.db.WithContext(ctx).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]mounts.Mount, len(rows))
	for i, r := range rows {
		out[i] = r.mount()
	}
	return out, nil
}

// GetMount returns the record named name, or mounts.ErrNotFound.
func (s *MountStore) GetMount(ctx context.Context, name string) (mounts.Mount, error) {
	var r mountRow
	err := s.db.WithContext(ctx).Where("name = ?", name).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mounts.Mount{}, fmt.Errorf("%w: mount %q", mounts.ErrNotFound, name)
	}
	if err != nil {
		return mounts.Mount{}, err
	}
	return r.mount(), nil
}

// CreateMount inserts m and bumps the generation counter in one transaction. The insert skips a
// name that exists, and a skipped insert returns mounts.ErrExists, so of two servers creating the
// same name, one wins. A bad name returns mounts.ErrInvalidArgument.
func (s *MountStore) CreateMount(ctx context.Context, m mounts.Mount) error {
	if err := mounts.ValidName(m.Name); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := toMountRow(m)
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("%w: mount %q", mounts.ErrExists, m.Name)
		}
		return bump(tx)
	})
}

// DeleteMount removes the record named name, and bumps the generation counter in the same
// transaction when there was one. Deleting a missing record is not an error.
func (s *MountStore) DeleteMount(ctx context.Context, name string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("name = ?", name).Delete(&mountRow{})
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		return bump(tx)
	})
}

func bump(tx *gorm.DB) error {
	return tx.Model(&generationRow{}).Where("id = ?", 1).
		UpdateColumn("counter", gorm.Expr("counter + ?", 1)).Error
}

// Generation returns the counter that every create and delete bumps, as a decimal string.
func (s *MountStore) Generation(ctx context.Context) (string, error) {
	var g generationRow
	if err := s.db.WithContext(ctx).Where("id = ?", 1).Take(&g).Error; err != nil {
		return "", err
	}
	return strconv.FormatInt(g.Counter, 10), nil
}

// connectionRow is a connection record's table row, keyed by owner and ID. It holds secret
// references, never secrets, since PutConnection validates first.
type connectionRow struct {
	Owner          string    `gorm:"column:owner;primaryKey;size:256"`
	ID             string    `gorm:"column:id;primaryKey;size:64"`
	Kind           string    `gorm:"column:kind;size:32"`
	Scope          string    `gorm:"column:scope;size:16"`
	Name           string    `gorm:"column:name"`
	BaseURL        string    `gorm:"column:base_url"`
	AppID          string    `gorm:"column:app_id;size:64"`
	InstallationID int64     `gorm:"column:installation_id"`
	KeyRef         string    `gorm:"column:key_ref"`
	TokenRef       string    `gorm:"column:token_ref"`
	Pasted         bool      `gorm:"column:pasted"`
	Status         string    `gorm:"column:status;size:16"`
	LastError      string    `gorm:"column:last_error"`
	Created        time.Time `gorm:"column:created"`
	LastUsed       time.Time `gorm:"column:last_used"`
	AccessKeyID    string    `gorm:"column:access_key_id;size:128"`
	RoleARN        string    `gorm:"column:role_arn"`
	Region         string    `gorm:"column:region;size:64"`
}

// ConnectionTable is the table connection records live in. GORM can't change a table's primary
// key, so keying records by owner and ID took a new table. Before goapplib 0.7.23 they were in
// OldConnectionTable, keyed by ID alone.
const ConnectionTable = "goapplib_connections_v2"

// OldConnectionTable is where connection records were before goapplib 0.7.23. NewConnectionStore
// copies its rows into ConnectionTable once and leaves it in place, so an older server can still
// read it during a rollout. Drop it by hand once every server runs 0.7.23 or later.
const OldConnectionTable = "goapplib_connections"

func (connectionRow) TableName() string { return ConnectionTable }

// oldConnectionRow is a row of OldConnectionTable. Its columns match connectionRow's.
type oldConnectionRow connectionRow

func (oldConnectionRow) TableName() string { return OldConnectionTable }

func toConnectionRow(c connections.Connection) connectionRow {
	return connectionRow{c.Owner, c.ID, string(c.Kind), string(c.Scope), c.Name, c.BaseURL, c.AppID,
		c.InstallationID, c.KeyRef, c.TokenRef, c.Pasted, string(c.Status), c.LastError, c.Created.UTC(), c.LastUsed.UTC(), c.AccessKeyID, c.RoleARN, c.Region}
}

func (r connectionRow) connection() connections.Connection {
	return connections.Connection{ID: r.ID, Kind: connections.Kind(r.Kind), Scope: connections.Scope(r.Scope),
		Owner: r.Owner, Name: r.Name, BaseURL: r.BaseURL, AppID: r.AppID, InstallationID: r.InstallationID,
		KeyRef: r.KeyRef, TokenRef: r.TokenRef, Pasted: r.Pasted, Status: connections.Status(r.Status), LastError: r.LastError,
		Created: r.Created.UTC(), LastUsed: r.LastUsed.UTC(), AccessKeyID: r.AccessKeyID, RoleARN: r.RoleARN, Region: r.Region}
}

// ConnectionStore is a connections.Store over a GORM database.
type ConnectionStore struct{ db *gorm.DB }

// NewConnectionStore migrates ConnectionTable in db and returns a store over it. When
// ConnectionTable is empty and OldConnectionTable exists, it copies the old table's rows in one
// transaction first, so records saved before goapplib 0.7.23 read back as they were.
func NewConnectionStore(db *gorm.DB) (*ConnectionStore, error) {
	if err := db.AutoMigrate(&connectionRow{}); err != nil {
		return nil, err
	}
	if db.Migrator().HasTable(OldConnectionTable) {
		err := db.Transaction(func(tx *gorm.DB) error {
			var n int64
			if err := tx.Model(&connectionRow{}).Count(&n).Error; err != nil || n > 0 {
				return err
			}
			var old []oldConnectionRow
			if err := tx.Find(&old).Error; err != nil || len(old) == 0 {
				return err
			}
			rows := make([]connectionRow, len(old))
			for i, r := range old {
				rows[i] = connectionRow(r)
			}
			return tx.CreateInBatches(rows, 100).Error
		})
		if err != nil {
			return nil, fmt.Errorf("gormstore: copying %s into %s: %w", OldConnectionTable, ConnectionTable, err)
		}
	}
	return &ConnectionStore{db: db}, nil
}

// GetConnection returns owner's record with that ID, or connections.ErrNotFound.
func (s *ConnectionStore) GetConnection(ctx context.Context, owner, id string) (connections.Connection, error) {
	var r connectionRow
	err := s.db.WithContext(ctx).Where("owner = ? AND id = ?", owner, id).Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connections.Connection{}, fmt.Errorf("%w: connection %q", connections.ErrNotFound, id)
	}
	if err != nil {
		return connections.Connection{}, err
	}
	return r.connection(), nil
}

// ListConnections returns the connections owned by owner, in ID order. An empty owner lists the
// server-scoped ones.
func (s *ConnectionStore) ListConnections(ctx context.Context, owner string) ([]connections.Connection, error) {
	var rows []connectionRow
	if err := s.db.WithContext(ctx).Where("owner = ?", owner).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]connections.Connection, len(rows))
	for i, r := range rows {
		out[i] = r.connection()
	}
	return out, nil
}

// PutConnection validates c, then inserts it or replaces the row with c.Owner and c.ID. An invalid
// record returns connections.ErrInvalid and writes nothing.
func (s *ConnectionStore) PutConnection(ctx context.Context, c connections.Connection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	row := toConnectionRow(c)
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

// DeleteConnection removes owner's row with that ID. Deleting a missing row is not an error.
func (s *ConnectionStore) DeleteConnection(ctx context.Context, owner, id string) error {
	return s.db.WithContext(ctx).Where("owner = ? AND id = ?", owner, id).Delete(&connectionRow{}).Error
}

var (
	_ mounts.RecordStore = (*MountStore)(nil)
	_ connections.Store  = (*ConnectionStore)(nil)
)
