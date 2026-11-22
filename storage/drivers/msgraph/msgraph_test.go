package msgraph_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/connections/entratest"
	"github.com/panyam/goapplib/storage/connections/fsstore"
	"github.com/panyam/goapplib/storage/drivers/msgraph"
	"github.com/panyam/goapplib/storage/drivers/msgraph/graphtest"
	"github.com/panyam/goapplib/storage/mounts"
	mountsfs "github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
	"golang.org/x/oauth2"
)

const (
	site    = "contoso.sharepoint.com,1111,2222"
	driveID = "b!boards"
	source  = "msgraph://sites/" + site + "/drives/" + driveID + "/root/Boards"
)

var creds = mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"}))

func fake(t *testing.T, versions bool) (*graphtest.Server, *graphtest.Drive) {
	t.Helper()
	g := graphtest.New(t)
	g.AddToken("tok")
	d := g.AddSiteDrive(site, driveID)
	d.KeepVersions(versions)
	g.SetMe(driveID)
	return g, d
}

func newDriver(t *testing.T, g *graphtest.Server) *msgraph.Driver {
	t.Helper()
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return msgraph.New(store, msgraph.WithBaseURL(g.BaseURL()))
}

func TestDriverContract(t *testing.T) {
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
				d.Put("Elsewhere/secret.txt", "outside the mount")
				return mountstest.DriverFixture{
					Driver:                newDriver(t, g),
					Source:                source,
					Missing:               "msgraph://drives/b!nope/root/Boards",
					Creds:                 creds,
					Change:                func(t *testing.T, p, data string) { d.Put("Boards/"+p, data) },
					SnapshotFailsOnChange: !versions,
				}
			})
		})
	}
}

func TestMeDriveSourceReads(t *testing.T) {
	g, d := fake(t, true)
	d.Put("Boards/README.md", "the jetson baseboard")
	drv := newDriver(t, g)
	ctx := context.Background()
	m := mounts.Mount{Name: "mine", Kind: mounts.KindMSGraph, Source: "msgraph://me/drive/root/Boards", Mode: mounts.ModeLive}
	fsys, err := drv.Open(ctx, m, creds)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := fs.ReadFile(fsys, "README.md"); err != nil || string(b) != "the jetson baseboard" {
		t.Errorf("ReadFile = %q, %v", b, err)
	}
	for _, src := range []string{"msgraph://me/files/Boards", "msgraph://sites/x/lists/y/root", "msgraph://users/alice/drive/root"} {
		if _, err := drv.Resolve(ctx, src, "", creds); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("Resolve(%q) = %v, want ErrInvalidArgument", src, err)
		}
	}
}

func TestDeltaReportsOneChangedFile(t *testing.T) {
	g, d := fake(t, true)
	d.Put("Boards/README.md", "v1")
	d.Put("Boards/board/main.kicad_sch", "(kicad_sch v1)")
	drv := newDriver(t, g)
	ctx := context.Background()
	m := mounts.Mount{Name: "boards", Kind: mounts.KindMSGraph, Source: source, Mode: mounts.ModeLive}

	start, err := drv.Delta(ctx, m, creds, "")
	if err != nil || len(start.Items) != 0 || start.Token == "" {
		t.Fatalf("Delta(\"\") = %+v, %v; want no items and a token", start, err)
	}
	d.Put("Boards/README.md", "v2")
	d.Put("Elsewhere/other.txt", "not under the mount")
	page, err := drv.Delta(ctx, m, creds, start.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Path != "README.md" || page.Items[0].Deleted || page.Token == start.Token {
		t.Fatalf("Delta = %+v; want README.md alone and a new token", page)
	}

	d.Delete("Boards/board/main.kicad_sch")
	gone, err := drv.Delta(ctx, m, creds, page.Token)
	if err != nil || len(gone.Items) != 1 || !gone.Items[0].Deleted || gone.Items[0].ItemID == "" {
		t.Errorf("Delta after a delete = %+v, %v; want one deleted item with its id", gone, err)
	}
	if _, err := drv.Delta(ctx, m, creds, "forgotten"); !errors.Is(err, msgraph.ErrResyncRequired) {
		t.Errorf("Delta(forgotten token) = %v, want ErrResyncRequired", err)
	}
}

func TestSubscriptionLifecycleAndNotification(t *testing.T) {
	g, _ := fake(t, true)
	drv := newDriver(t, g)
	ctx := context.Background()
	m := mounts.Mount{Name: "boards", Kind: mounts.KindMSGraph, Source: source, Mode: mounts.ModeLive}
	expires := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	sub, err := drv.CreateSubscription(ctx, m, creds, "https://app.example.com/graph-hook", "s3cret-state", expires)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Subscriptions()[sub.ID]; got.Resource != "sites/"+site+"/drives/"+driveID+"/root" || got.ClientState != "s3cret-state" {
		t.Errorf("subscription = %+v; want the drive root and the client state", got)
	}
	later := expires.Add(48 * time.Hour)
	if renewed, err := drv.RenewSubscription(ctx, creds, sub.ID, later); err != nil || !renewed.Expires.Equal(later) {
		t.Errorf("RenewSubscription = %+v, %v; want expiry %v", renewed, err, later)
	}

	body := g.NotificationBody(sub.ID)
	changes, err := msgraph.ParseNotification(httptest.NewRequest(http.MethodPost, "/graph-hook", bytes.NewReader(body)), "s3cret-state")
	if err != nil || len(changes) != 1 || changes[0].Source != "msgraph://sites/"+site+"/drives/"+driveID+"/root" || !changes[0].Default {
		t.Errorf("ParseNotification = %+v, %v; want one change to the drive root", changes, err)
	}
	if _, err := msgraph.ParseNotification(httptest.NewRequest(http.MethodPost, "/graph-hook", bytes.NewReader(body)), "other"); !errors.Is(err, msgraph.ErrBadClientState) {
		t.Errorf("ParseNotification(wrong state) = %v, want ErrBadClientState", err)
	}

	rec := httptest.NewRecorder()
	if !msgraph.AnswerValidation(rec, httptest.NewRequest(http.MethodPost, "/graph-hook?validationToken=%3Cscript%3Ex", nil)) {
		t.Fatal("AnswerValidation didn't see the validation token")
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "<script>x" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("validation answer = %d %q %q; want 200 echoing the token as text/plain", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if msgraph.AnswerValidation(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/graph-hook", nil)) {
		t.Error("AnswerValidation answered a request with no validation token")
	}

	if err := drv.DeleteSubscription(ctx, creds, sub.ID); err != nil || len(g.Subscriptions()) != 0 {
		t.Errorf("DeleteSubscription = %v, left %v", err, g.Subscriptions())
	}
	if err := drv.DeleteSubscription(ctx, creds, sub.ID); err != nil {
		t.Errorf("deleting a gone subscription = %v, want nil", err)
	}
}

func TestNextLinkOffGraphsHostIsRefused(t *testing.T) {
	var gotToken string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	t.Cleanup(elsewhere.Close)
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"1","name":"a.txt","file":{},"size":1}],"@odata.nextLink":"` + elsewhere.URL + `/steal"}`))
	}))
	t.Cleanup(graph.Close)
	store, _ := blobs.NewDir(t.TempDir())
	drv := msgraph.New(store, msgraph.WithBaseURL(graph.URL+"/v1.0"))
	if _, err := drv.Resolve(context.Background(), "msgraph://drives/d/root/Boards", "", creds); !errors.Is(err, mounts.ErrUnavailable) {
		t.Errorf("Resolve = %v, want ErrUnavailable for a link off Graph's host", err)
	}
	if gotToken != "" {
		t.Errorf("the other host got Authorization %q", gotToken)
	}
}

func TestReadThroughAnEntraConnection(t *testing.T) {
	ent := entratest.New(t)
	ent.AddSecretApp("contoso.onmicrosoft.com", "app-1", "s3cret")
	g, d := fake(t, true)
	g.AcceptToken(ent.ValidToken)
	d.Put("Boards/README.md", "the jetson baseboard")

	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOAPPLIB_TEST_GRAPH_SECRET", "s3cret")
	if err := store.PutConnection(context.Background(), connections.Connection{ID: "sp", Kind: connections.KindEntra, Scope: connections.ScopeServer,
		BaseURL: ent.URL, TenantID: "contoso.onmicrosoft.com", ClientID: "app-1", EntraAuth: connections.EntraSecret, KeyRef: "env:GOAPPLIB_TEST_GRAPH_SECRET"}); err != nil {
		t.Fatal(err)
	}
	svc := connections.NewService(store)
	blobStore, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mstore, err := mountsfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(mstore, []mounts.Driver{msgraph.New(blobStore, msgraph.WithBaseURL(g.BaseURL()))}, mounts.WithCredentials(svc.Credentials))
	ctx := context.Background()
	if _, err := reg.Create(ctx, mounts.Mount{Name: "boards", Kind: mounts.KindMSGraph, Source: source, Mode: mounts.ModeSnapshot, Connection: "sp"}); err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(reg.FS(ctx), "boards/README.md")
	if err != nil || string(b) != "the jetson baseboard" {
		t.Errorf("ReadFile = %q, %v", b, err)
	}
}

// TestLiveMSGraph reads a real drive. It skips unless STORAGE_LIVE_MSGRAPH names a source, such
// as "msgraph://me/drive/root/Boards", and STORAGE_LIVE_MSGRAPH_TOKEN holds a Graph access token
// that can read it ("az account get-access-token --resource https://graph.microsoft.com" gives
// one).
func TestLiveMSGraph(t *testing.T) {
	src, tok := os.Getenv("STORAGE_LIVE_MSGRAPH"), os.Getenv("STORAGE_LIVE_MSGRAPH_TOKEN")
	if src == "" || tok == "" {
		t.Skip("set STORAGE_LIVE_MSGRAPH=msgraph://... and STORAGE_LIVE_MSGRAPH_TOKEN to read a real drive")
	}
	live := mounts.Bearer(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tok}))
	store, err := blobs.NewDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drv := msgraph.New(store)
	ctx := context.Background()
	v, err := drv.Resolve(ctx, src, "", live)
	if err != nil {
		t.Fatal(err)
	}
	m, err := drv.Prepare(ctx, mounts.Mount{Name: "live", Kind: mounts.KindMSGraph, Source: src, Version: v, Mode: mounts.ModeSnapshot}, live)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := drv.Open(ctx, m, live)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	err = fs.WalkDir(fsys, ".", func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || n >= 3 {
			return err
		}
		n++
		b, err := fs.ReadFile(fsys, p)
		t.Logf("%s: %d bytes", p, len(b))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := drv.Delta(ctx, m, live, "")
	if err != nil || page.Token == "" {
		t.Errorf("Delta(\"\") = %+v, %v; want a token", page, err)
	}
}

func TestFollowingFindsEveryMountInTheNotifiedDrive(t *testing.T) {
	g, d := fake(t, true)
	d.Put("Boards/README.md", "the jetson baseboard")
	d.Put("Elsewhere/notes.md", "x")
	g.AddSiteDrive(site, "b!other").Put("Boards/README.md", "another drive")
	ctx := context.Background()
	store, err := mountsfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{newDriver(t, g)},
		mounts.WithCredentials(func(context.Context, string, string) (mounts.Credentials, error) { return creds, nil }))
	drive := "msgraph://sites/" + site + "/drives/"
	for name, src := range map[string]string{"boards": source, "elsewhere": drive + driveID + "/root/Elsewhere", "other": drive + "b!other/root/Boards"} {
		if _, err := reg.Create(ctx, mounts.Mount{Name: name, Kind: mounts.KindMSGraph, Source: src, Mode: mounts.ModeLive, Connection: "graph"}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	m := mounts.Mount{Name: "boards", Kind: mounts.KindMSGraph, Source: source, Mode: mounts.ModeLive}
	sub, err := newDriver(t, g).CreateSubscription(ctx, m, creds, "https://app.example.com/graph-hook", "state", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	changes, err := msgraph.ParseNotification(httptest.NewRequest(http.MethodPost, "/graph-hook", bytes.NewReader(g.NotificationBody(sub.ID))), "state")
	if err != nil || len(changes) != 1 {
		t.Fatalf("ParseNotification = %+v, %v", changes, err)
	}
	ms, err := reg.Following(ctx, mounts.KindMSGraph, changes[0])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.Name)
	}
	slices.Sort(got)
	if want := []string{"boards", "elsewhere"}; !slices.Equal(got, want) {
		t.Errorf("Following(drive root change) = %v, want %v: every mount in that drive, none in another", got, want)
	}
}
