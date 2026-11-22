package github_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
)

func names(rs []mounts.Ref) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func TestTagsListNewestFirstByCommitDate(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("tok")
	ts := static("tok")
	c1 := e.repo.Head("main")
	c2 := e.repo.Commit("main", map[string]string{"x": "2"}, nil)
	c3 := e.repo.Commit("main", map[string]string{"x": "3"}, nil)
	// By name v1.9 sorts after v1.10, but v1.10 is the newer commit.
	e.repo.Tag("v1.9", c2, false)
	e.repo.Tag("v1.10", c3, true)
	e.repo.Tag("v1.0", c1, false)
	when := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	e.repo.SetCommitTime(c3, when)

	page, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag, PageSize: 2}, ts)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(page.Refs), []string{"v1.10", "v1.9"}; !slices.Equal(got, want) {
		t.Errorf("first page = %v, want %v", got, want)
	}
	top := page.Refs[0]
	if top.Kind != mounts.RefTag || top.Version != c3 || !top.Time.Equal(when) || !strings.HasPrefix(top.Message, "commit 3") {
		t.Errorf("annotated tag = %+v; want its commit %s, dated %v, with the commit's first line", top, c3, when)
	}
	if page.NextPageToken == "" {
		t.Fatal("first page has no next token, want one for v1.0")
	}
	page, err = e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag, PageSize: 2, PageToken: page.NextPageToken}, ts)
	if err != nil || !slices.Equal(names(page.Refs), []string{"v1.0"}) || page.NextPageToken != "" || page.Refs[0].Version != c1 {
		t.Errorf("second page = %+v, %v; want v1.0 at %s, and no next token", page, err, c1)
	}

	// GitHub orders tags from one day by name, and the driver puts them in order by time.
	c4 := e.repo.Commit("main", map[string]string{"x": "4"}, nil)
	e.repo.SetCommitTime(c4, when.Add(time.Hour))
	e.repo.Tag("v1.1.5", c4, false)
	page, err = e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag, PageSize: 2}, ts)
	if got, want := names(page.Refs), []string{"v1.1.5", "v1.10"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("two tags from one day = %v, %v; want %v, by time", got, err, want)
	}

	// The repo is private, and GitHub's REST tags API answers 404 to a request with no token.
	if _, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag}, nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("a private repo's tags with no token = %v, want ErrNotFound", err)
	}
	if _, err := e.drv.ListRefs(e.ctx, "github://acme/nope", mounts.ListRefsRequest{Kind: mounts.RefTag}, ts); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("tags of a missing repo = %v, want ErrNotFound", err)
	}
}

func TestABranchListsItsCommitsNewestFirst(t *testing.T) {
	e := newEnv(t, false)
	c1 := e.repo.Head("main")
	c2 := e.repo.Commit("main", map[string]string{"x": "2"}, nil)
	c3 := e.repo.Commit("main", map[string]string{"x": "3"}, nil)
	side := e.repo.Commit("side", map[string]string{"y": "1"}, nil)

	req := mounts.ListRefsRequest{Kind: mounts.RefCommit, PageSize: 2}
	page, err := e.drv.ListRefs(e.ctx, e.repo.Source(), req, nil)
	if err != nil || !slices.Equal(names(page.Refs), []string{c3, c2}) || page.NextPageToken == "" {
		t.Fatalf("default branch, first page = %v, %q, %v; want %v and a next token", names(page.Refs), page.NextPageToken, err, []string{c3, c2})
	}
	if r := page.Refs[0]; r.Kind != mounts.RefCommit || r.Version != c3 || r.Time.IsZero() || r.Message != "commit 3 on main" {
		t.Errorf("a commit ref = %+v; want its sha, date and first line", r)
	}
	req.PageToken = page.NextPageToken
	page, err = e.drv.ListRefs(e.ctx, e.repo.Source(), req, nil)
	if err != nil || !slices.Equal(names(page.Refs), []string{c1}) || page.NextPageToken != "" {
		t.Errorf("second page = %v, %q, %v; want %s and no next token", names(page.Refs), page.NextPageToken, err, c1)
	}
	page, err = e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefCommit, Branch: "side"}, nil)
	if err != nil || len(page.Refs) == 0 || page.Refs[0].Name != side {
		t.Errorf("branch side = %v, %v; want %s first", names(page.Refs), err, side)
	}
	if _, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefCommit, Branch: "nope"}, nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("a missing branch = %v, want ErrNotFound", err)
	}
	if _, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefCommit, PageToken: "x"}, nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("a bad page token = %v, want ErrInvalidArgument", err)
	}
}

func TestAPrivateRepoListsRefsOnlyWithItsToken(t *testing.T) {
	e := newEnv(t, true)
	if _, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefCommit}, nil); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("a private repo's commits with no token = %v, want ErrNotFound", err)
	}
	e.gh.AddToken("tok")
	if page, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefCommit}, static("tok")); err != nil || len(page.Refs) != 1 {
		t.Errorf("with the token = %v, %v; want the one commit", names(page.Refs), err)
	}
	for _, r := range e.gh.Requests() {
		if r.Path == "/graphql" || strings.HasSuffix(r.Path, "/commits") && r.Token != "" && r.Token != "tok" {
			t.Errorf("request %+v carried the wrong token", r)
		}
	}
}

type recordURL struct{ got *string }

func (r recordURL) RoundTrip(req *http.Request) (*http.Response, error) {
	*r.got = req.URL.String()
	return nil, errors.New("not sent")
}

func TestGraphQLLivesBesideTheRESTBase(t *testing.T) {
	for base, want := range map[string]string{
		"https://ghe.example.com/api/v3": "https://ghe.example.com/api/graphql",
		github.DefaultBaseURL:            "https://api.github.com/graphql",
	} {
		var got string
		d := github.New(nil, github.WithBaseURL(base), github.WithHTTPClient(&http.Client{Transport: recordURL{&got}}))
		d.ListRefs(context.Background(), "github://acme/board", mounts.ListRefsRequest{Kind: mounts.RefTag}, static("tok"))
		if got != want {
			t.Errorf("tags with API base %s went to %q, want %q", base, got, want)
		}
	}
}

func TestSameSourceIgnoresCaseAndDotGit(t *testing.T) {
	d := github.New(nil)
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"github://acme/board", "github://Acme/Board", true},
		{"github://acme/board", "github://acme/board.git", true},
		{"github://acme/board", "github://acme/boards", false},
		{"github://acme/board", "not a source", false},
	} {
		if got := d.SameSource(c.a, c.b); got != c.want {
			t.Errorf("SameSource(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAPushReachesTheMountsFollowingItThroughTheRegistry(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("tok")
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{e.drv}, mounts.WithCredentials(func(context.Context, string, string) (mounts.Credentials, error) {
		return static("tok"), nil
	}))
	// The record says Acme, and GitHub's push says acme.
	if _, err := reg.Create(e.ctx, mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: "github://Acme/Board", Mode: mounts.ModeLive, Connection: "c"}); err != nil {
		t.Fatal(err)
	}
	e.repo.Tag("v1", e.repo.Head("main"), false)
	if page, err := reg.ListRefs(e.ctx, "board", mounts.ListRefsRequest{Kind: mounts.RefTag}); err != nil || !slices.Equal(names(page.Refs), []string{"v1"}) {
		t.Errorf("Registry.ListRefs = %v, %v; want v1 through the mount's connection", names(page.Refs), err)
	}

	head := e.repo.Commit("main", map[string]string{"x": "1"}, nil)
	p, err := github.ParsePush(e.repo.PushRequest("/hook", []byte("k"), "main"), []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := reg.Following(e.ctx, mounts.KindGitHub, p.Change)
	if err != nil || len(ms) != 1 || ms[0].Name != "board" {
		t.Fatalf("Following = %v, %v; want board", ms, err)
	}
	if v, _, err := reg.ResolveAt(e.ctx, "board", ms[0].Ref); err != nil || v != head || v != p.Version {
		t.Errorf("ResolveAt after the push = %q, %v; want the pushed %s", v, err, head)
	}
}

func TestAPublicRepoListsTagsWithNoConnection(t *testing.T) {
	e := newEnv(t, false)
	c1 := e.repo.Head("main")
	c2 := e.repo.Commit("main", map[string]string{"x": "2"}, nil)
	when := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	e.repo.SetCommitTime(c2, when)
	// GitHub's REST tags API orders by version, so v1.10 comes before v1.9.
	e.repo.Tag("v1.10", c2, true)
	for i := 0; i <= 9; i++ {
		e.repo.Tag(fmt.Sprintf("v1.%d", i), c1, false)
	}

	page, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"v1.10", "v1.9", "v1.8", "v1.7", "v1.6", "v1.5", "v1.4", "v1.3", "v1.2", "v1.1"}
	if got := names(page.Refs); !slices.Equal(got, want) {
		t.Errorf("first page = %v, want %v (ten, the most a page holds with no connection)", got, want)
	}
	top := page.Refs[0]
	if top.Kind != mounts.RefTag || top.Version != c2 || !top.Time.Equal(when) || !strings.HasPrefix(top.Message, "commit 2") {
		t.Errorf("annotated tag = %+v; want its commit %s, dated %v, with the commit's first line", top, c2, when)
	}
	if page.NextPageToken == "" {
		t.Fatal("first page has no next token, want one for v1.0")
	}
	page, err = e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag, PageToken: page.NextPageToken}, nil)
	if err != nil || !slices.Equal(names(page.Refs), []string{"v1.0"}) || page.NextPageToken != "" || page.Refs[0].Version != c1 || page.Refs[0].Time.IsZero() {
		t.Errorf("second page = %+v, %v; want v1.0 at %s with its date, and no next token", page, err, c1)
	}
	if _, err := e.drv.ListRefs(e.ctx, e.repo.Source(), mounts.ListRefsRequest{Kind: mounts.RefTag, PageToken: "x"}, nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("a bad page token = %v, want ErrInvalidArgument", err)
	}
	for _, r := range e.gh.Requests() {
		if r.Path == "/graphql" {
			t.Errorf("listing with no connection sent %+v, want only REST requests", r)
		}
	}
}

// rateLimited answers every request as GitHub does once a caller with no token has used its
// requests for the hour.
type rateLimited struct{}

func (rateLimited) RoundTrip(req *http.Request) (*http.Response, error) {
	h := http.Header{"X-Ratelimit-Remaining": {"0"}, "Content-Type": {"application/json"}}
	return &http.Response{StatusCode: http.StatusForbidden, Header: h, Request: req,
		Body: io.NopCloser(strings.NewReader(`{"message":"API rate limit exceeded for 203.0.113.7."}`))}, nil
}

func TestTagsWithNoConnectionSayAConnectionLiftsTheRateLimit(t *testing.T) {
	d := github.New(nil, github.WithHTTPClient(&http.Client{Transport: rateLimited{}}))
	_, err := d.ListRefs(context.Background(), "github://acme/board", mounts.ListRefsRequest{Kind: mounts.RefTag}, nil)
	if !errors.Is(err, mounts.ErrUnavailable) || !strings.Contains(err.Error(), "connection") {
		t.Errorf("rate-limited tags = %v, want ErrUnavailable saying a connection lifts the limit", err)
	}
}
