package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// refsPageSize is the default page size of ListRefs, and refsMaxPage the most GitHub returns in one
// page of either API. anonTagsPage is the most tags a page holds with no connection, since each
// costs a request against GitHub's limit of 60 an hour without a token.
const (
	refsPageSize = 30
	refsMaxPage  = 100
	anonTagsPage = 10
)

// tagsQuery lists a repo's tags newest first by their commit's date. An annotated tag's target is
// a tag object, whose own target is the commit.
const tagsQuery = `query($owner: String!, $name: String!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    refs(refPrefix: "refs/tags/", first: $first, after: $after, orderBy: {field: TAG_COMMIT_DATE, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes {
        name
        target {
          __typename
          ... on Commit { oid committedDate messageHeadline }
          ... on Tag { target { __typename ... on Commit { oid committedDate messageHeadline } } }
        }
      }
    }
  }
}`

// ListRefs lists the repo's tags, or a branch's commits, newest first.
//
// With a connection, tags come from one GraphQL query per page, ordered by their commit's date.
// GitHub orders by the commit's day only, then by name, so the driver sorts each page by time. Two
// tags made the same day on either side of a page break can still come out of order. A tag on
// something other than a commit, such as a tree, is left out.
//
// GitHub's GraphQL API refuses a request with no token, so without a connection tags come from
// the REST tags API, which GitHub orders by version, newest first (v1.10 before v1.9). It gives no
// dates, so the driver reads each tag's commit too. Every request counts against GitHub's limit of
// 60 an hour without a token, so a page holds at most 10 tags, and a refusal for the limit returns
// ErrUnavailable saying a connection lifts it.
//
// Commits come from the REST commits API, following parents from the branch's head. An empty
// Branch is the default branch. A missing branch returns ErrNotFound.
func (d *Driver) ListRefs(ctx context.Context, source string, req mounts.ListRefsRequest, ts oauth2.TokenSource) (mounts.RefPage, error) {
	rp, err := repo(source)
	if err != nil {
		return mounts.RefPage{}, err
	}
	size := req.PageSize
	if size <= 0 {
		size = refsPageSize
	}
	size = min(size, refsMaxPage)
	switch req.Kind {
	case mounts.RefTag:
		return d.listTags(ctx, rp, size, req.PageToken, ts)
	case mounts.RefCommit:
		return d.listCommits(ctx, rp, req.Branch, size, req.PageToken, ts)
	}
	return mounts.RefPage{}, fmt.Errorf("%w: GitHub lists tags or commits, not %q", mounts.ErrInvalidArgument, req.Kind)
}

// pageNumber reads a REST page token, which is the page's number. Empty is the first page.
func pageNumber(token string) (int, error) {
	if token == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(token)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%w: page token %q", mounts.ErrInvalidArgument, token)
	}
	return n, nil
}

func (d *Driver) listCommits(ctx context.Context, rp, branch string, size int, token string, ts oauth2.TokenSource) (mounts.RefPage, error) {
	page, err := pageNumber(token)
	if err != nil {
		return mounts.RefPage{}, err
	}
	q := url.Values{"per_page": {strconv.Itoa(size)}, "page": {strconv.Itoa(page)}}
	if branch != "" {
		q.Set("sha", branch)
	}
	resp, err := d.send(ctx, ts, http.MethodGet, rp+"/commits", q, nil)
	if err != nil {
		return mounts.RefPage{}, err
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return mounts.RefPage{}, err
	}
	var cs []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cs); err != nil {
		return mounts.RefPage{}, fmt.Errorf("%w: decoding %s/commits: %v", mounts.ErrUnavailable, rp, err)
	}
	var out mounts.RefPage
	for _, c := range cs {
		if !commitSHA.MatchString(c.SHA) {
			return mounts.RefPage{}, fmt.Errorf("%w: GitHub listed commit %q", mounts.ErrUnavailable, c.SHA)
		}
		headline, _, _ := strings.Cut(c.Commit.Message, "\n")
		out.Refs = append(out.Refs, mounts.Ref{Name: c.SHA, Kind: mounts.RefCommit, Version: c.SHA, Time: c.Commit.Committer.Date, Message: headline})
	}
	if hasNext(resp.Header.Get("Link")) {
		out.NextPageToken = strconv.Itoa(page + 1)
	}
	return out, nil
}

// hasNext reports whether a Link header names a next page.
func hasNext(link string) bool {
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}

type gqlCommit struct {
	Typename        string    `json:"__typename"`
	OID             string    `json:"oid"`
	CommittedDate   time.Time `json:"committedDate"`
	MessageHeadline string    `json:"messageHeadline"`
}

func (d *Driver) listTags(ctx context.Context, rp string, size int, token string, ts oauth2.TokenSource) (mounts.RefPage, error) {
	if ts == nil {
		return d.listTagsREST(ctx, rp, min(size, anonTagsPage), token)
	}
	owner, name, _ := strings.Cut(strings.TrimPrefix(rp, "/repos/"), "/")
	vars := map[string]any{"owner": owner, "name": name, "first": size}
	if token != "" {
		vars["after"] = token
	}
	b, err := json.Marshal(map[string]any{"query": tagsQuery, "variables": vars})
	if err != nil {
		return mounts.RefPage{}, err
	}
	resp, err := d.sendURL(ctx, ts, http.MethodPost, d.graphqlURL(), bytes.NewReader(b), true)
	if err != nil {
		return mounts.RefPage{}, err
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return mounts.RefPage{}, err
	}
	var r struct {
		Data struct {
			Repository *struct {
				Refs struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Name   string `json:"name"`
						Target struct {
							gqlCommit
							Target *gqlCommit `json:"target"`
						} `json:"target"`
					} `json:"nodes"`
				} `json:"refs"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return mounts.RefPage{}, fmt.Errorf("%w: decoding GitHub's tags of %s: %v", mounts.ErrUnavailable, rp, err)
	}
	if r.Data.Repository == nil {
		for _, e := range r.Errors {
			if e.Type == "NOT_FOUND" {
				return mounts.RefPage{}, fmt.Errorf("%w: %s", mounts.ErrNotFound, e.Message)
			}
		}
		return mounts.RefPage{}, fmt.Errorf("%w: GitHub listed no tags of %s: %v", mounts.ErrUnavailable, rp, r.Errors)
	}
	var out mounts.RefPage
	refs := r.Data.Repository.Refs
	for _, n := range refs.Nodes {
		c := n.Target.gqlCommit
		if c.Typename == "Tag" && n.Target.Target != nil {
			c = *n.Target.Target
		}
		if c.Typename != "Commit" || !commitSHA.MatchString(c.OID) {
			continue
		}
		out.Refs = append(out.Refs, mounts.Ref{Name: n.Name, Kind: mounts.RefTag, Version: c.OID, Time: c.CommittedDate, Message: c.MessageHeadline})
	}
	// GitHub orders by the commit's day, then by name, so a page's tags from one day are put in
	// order by time here.
	sort.SliceStable(out.Refs, func(i, j int) bool { return out.Refs[i].Time.After(out.Refs[j].Time) })
	if refs.PageInfo.HasNextPage {
		out.NextPageToken = refs.PageInfo.EndCursor
	}
	return out, nil
}

// graphqlURL is GitHub's GraphQL endpoint for the driver's API base: api.github.com/graphql for
// GitHub, <host>/api/graphql for GitHub Enterprise's <host>/api/v3, and <base>/graphql otherwise,
// such as for a test's fake.
func (d *Driver) graphqlURL() string {
	if b, ok := strings.CutSuffix(d.base, "/api/v3"); ok {
		return b + "/api/graphql"
	}
	return d.base + "/graphql"
}

// SameSource reports whether a and b name one repo. GitHub ignores case in owner and repo names,
// and a trailing ".git" names the same repo.
func (d *Driver) SameSource(a, b string) bool {
	ra, err := repo(a)
	if err != nil {
		return false
	}
	rb, err := repo(b)
	return err == nil && strings.EqualFold(ra, rb)
}

var (
	_ mounts.RefLister     = (*Driver)(nil)
	_ mounts.SourceMatcher = (*Driver)(nil)
)

// listTagsREST lists a page of tags with no token: the REST tags API for names and commits, then
// one commits request per tag for its date and message.
func (d *Driver) listTagsREST(ctx context.Context, rp string, size int, token string) (mounts.RefPage, error) {
	page, err := pageNumber(token)
	if err != nil {
		return mounts.RefPage{}, err
	}
	var tags []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	link, err := d.anonGet(ctx, rp+"/tags", url.Values{"per_page": {strconv.Itoa(size)}, "page": {strconv.Itoa(page)}}, &tags)
	if err != nil {
		return mounts.RefPage{}, err
	}
	var out mounts.RefPage
	for _, t := range tags {
		if !commitSHA.MatchString(t.Commit.SHA) {
			return mounts.RefPage{}, fmt.Errorf("%w: GitHub listed tag %q at %q", mounts.ErrUnavailable, t.Name, t.Commit.SHA)
		}
		var c struct {
			Commit struct {
				Message   string `json:"message"`
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		if _, err := d.anonGet(ctx, rp+"/commits/"+t.Commit.SHA, nil, &c); err != nil {
			return mounts.RefPage{}, err
		}
		headline, _, _ := strings.Cut(c.Commit.Message, "\n")
		out.Refs = append(out.Refs, mounts.Ref{Name: t.Name, Kind: mounts.RefTag, Version: t.Commit.SHA, Time: c.Commit.Committer.Date, Message: headline})
	}
	if hasNext(link) {
		out.NextPageToken = strconv.Itoa(page + 1)
	}
	return out, nil
}

// anonGet sends a GET with no token and decodes the answer into out. It returns the Link header.
// A refusal for GitHub's limit on requests with no token says that a connection lifts it.
func (d *Driver) anonGet(ctx context.Context, p string, q url.Values, out any) (string, error) {
	resp, err := d.send(ctx, nil, http.MethodGet, p, q, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		limited := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests
		if limited && resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return "", fmt.Errorf("%w; a connection lifts GitHub's limit of 60 requests an hour without a token", err)
		}
		return "", err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return "", fmt.Errorf("%w: decoding %s: %v", mounts.ErrUnavailable, p, err)
	}
	return resp.Header.Get("Link"), nil
}
