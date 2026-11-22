package github_test

import (
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestDriverContract(t *testing.T) {
	mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
		gh := githubtest.New(t)
		repo := gh.AddRepo("acme", "board", true, files)
		repo.Grant("ghs_installation")
		b, err := blobs.NewDir(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		views := t.TempDir()
		return mountstest.DriverFixture{
			Driver:    github.New(b, github.WithBaseURL(gh.URL), github.WithRawURL(gh.RawURL()), github.WithViews(views)),
			HostPaths: []mounts.Mode{mounts.ModeSnapshot},
			Views:     views,
			Source:    repo.Source(),
			Missing:   "github://acme/nope",
			Creds:     static("ghs_installation"),
			Change: func(t *testing.T, p, data string) {
				repo.Commit("main", map[string]string{p: data}, nil)
			},
		}
	})
}
