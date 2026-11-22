package msgraph_test

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/panyam/goapplib/storage/drivers/msgraph"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestDriverContractWithViews(t *testing.T) {
	for _, versions := range []bool{true, false} {
		name := "no-versions"
		if versions {
			name = "versions"
		}
		t.Run(name, func(t *testing.T) {
			mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
				g, d := fake(t, versions)
				for p, data := range files {
					d.Put("Boards/"+p, data)
				}
				views, net := t.TempDir(), &switchable{}
				return mountstest.DriverFixture{
					Driver:    newDriver(t, g, msgraph.WithViews(views), msgraph.WithHTTPClient(&http.Client{Transport: net})),
					Source:    source,
					Missing:   "msgraph://drives/b!nope/root/Boards",
					Creds:     creds,
					Change:    func(t *testing.T, p, data string) { d.Put("Boards/"+p, data) },
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
