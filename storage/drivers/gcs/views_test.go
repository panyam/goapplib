package gcs_test

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/panyam/goapplib/storage/drivers/gcs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestDriverContractWithViews(t *testing.T) {
	for _, versioned := range []bool{true, false} {
		name := "unversioned"
		if versioned {
			name = "versioned"
		}
		t.Run(name, func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				srv := fake(t, versioned)
				for p, data := range files {
					put(srv, "boards/jetson/"+p, data)
				}
				views, net := t.TempDir(), &switchable{}
				return mountstest.DriverFixture{
					Driver:    newDriver(t, srv, gcs.WithViews(views), gcs.WithHTTPClient(&http.Client{Transport: net})),
					Source:    "gs://designs/boards/jetson",
					Missing:   "gs://nope/boards",
					Creds:     creds,
					Change:    func(t *testing.T, p, data string) { put(srv, "boards/jetson/"+p, data) },
					HostPaths: []mounts.Mode{mounts.ModeSnapshot},
					Views:     views,
					Offline:   func(*testing.T) { net.down.Store(true) },
				}
			})
		})
	}
}

// switchable fails every request once down is set, so a test can take the service away.
type switchable struct{ down atomic.Bool }

func (s *switchable) RoundTrip(r *http.Request) (*http.Response, error) {
	if s.down.Load() {
		return nil, errors.New("the service is offline")
	}
	return http.DefaultTransport.RoundTrip(r)
}
