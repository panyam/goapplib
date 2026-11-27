package datastore

import (
	"errors"
	"os"
	"testing"

	"cloud.google.com/go/datastore"
)

type fakeIndexProvider struct{}

func (fakeIndexProvider) ServiceName() string { return "widgets" }
func (fakeIndexProvider) RequiredIndexes() []DatastoreIndex {
	return []DatastoreIndex{{Kind: "Widget", Properties: []IndexProperty{{Name: "owner"}, {Name: "created", Direction: "desc"}}}}
}
func (fakeIndexProvider) TestQueries() []*datastore.Query { return nil }

func TestReportMissingIndexes_WarnWritesNoFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := reportMissingIndexes(errors.New("missing index"), fakeIndexProvider{}, ValidationWarn); err != nil {
		t.Fatalf("warn mode returned %v, want nil", err)
	}
	if _, err := os.Stat("widgets_index.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("warn mode wrote widgets_index.yaml into the working directory (stat err = %v)", err)
	}
}

func TestReportMissingIndexes_ErrorWritesFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := reportMissingIndexes(errors.New("missing index"), fakeIndexProvider{}, ValidationError); err == nil {
		t.Fatal("error mode returned nil")
	}
	if _, err := os.Stat("widgets_index.yaml"); err != nil {
		t.Errorf("error mode wrote no widgets_index.yaml: %v", err)
	}
}
