package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// hook is the part of GitHub's hook object the driver reads.
type hook struct {
	ID     int64    `json:"id"`
	Events []string `json:"events"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

// EnsureWebhook makes a hook on the repo that sends req.Events to req.URL as JSON, signed with
// req.Secret, or updates the hook already sending to req.URL. It always sends the update, since
// GitHub never shows a hook's secret, so the driver can't tell whether it changed. The JSON
// content type lets ParsePush read each delivery as sent.
//
// GitHub lets a token manage a repo's hooks only with admin rights on the repo, or a fine-grained
// token's webhooks permission. It answers 404 to a token that can read the repo but can't manage
// its hooks, so 404 and 403 return mounts.ErrPermissionDenied. 422, such as for a URL GitHub won't
// send to, returns mounts.ErrInvalidArgument with GitHub's message.
func (d *Driver) EnsureWebhook(ctx context.Context, source string, req mounts.WebhookRequest, ts oauth2.TokenSource) (mounts.Webhook, error) {
	rp, err := repo(source)
	if err != nil {
		return mounts.Webhook{}, err
	}
	hooks, err := d.listHooks(ctx, rp, ts)
	if err != nil {
		return mounts.Webhook{}, err
	}
	body := map[string]any{
		"active": true,
		"events": req.Events,
		"config": map[string]any{"url": req.URL, "content_type": "json", "secret": string(req.Secret), "insecure_ssl": "0"},
	}
	var out hook
	created := true
	for _, h := range hooks {
		if h.Config.URL == req.URL {
			created = false
			if err := d.hookCall(ctx, ts, http.MethodPatch, rp+"/hooks/"+strconv.FormatInt(h.ID, 10), body, &out); err != nil {
				return mounts.Webhook{}, err
			}
			break
		}
	}
	if created {
		body["name"] = "web"
		if err := d.hookCall(ctx, ts, http.MethodPost, rp+"/hooks", body, &out); err != nil {
			return mounts.Webhook{}, err
		}
	}
	return mounts.Webhook{ID: strconv.FormatInt(out.ID, 10), URL: out.Config.URL, Events: out.Events, Created: created}, nil
}

// DeleteWebhook removes the repo's hooks that send to url. It lists the hooks first, so a 404
// means the token can't manage them, which returns mounts.ErrPermissionDenied, and a hook that is
// already gone isn't an error.
func (d *Driver) DeleteWebhook(ctx context.Context, source, url string, ts oauth2.TokenSource) error {
	rp, err := repo(source)
	if err != nil {
		return err
	}
	hooks, err := d.listHooks(ctx, rp, ts)
	if err != nil {
		return err
	}
	for _, h := range hooks {
		if h.Config.URL == url {
			if err := d.hookCall(ctx, ts, http.MethodDelete, rp+"/hooks/"+strconv.FormatInt(h.ID, 10), nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// listHooks reads one page of 100 hooks. GitHub allows at most 20 hooks per event on a repo, so
// one page holds them all.
func (d *Driver) listHooks(ctx context.Context, rp string, ts oauth2.TokenSource) ([]hook, error) {
	var hooks []hook
	err := d.hookCall(ctx, ts, http.MethodGet, rp+"/hooks?per_page=100", nil, &hooks)
	return hooks, err
}

func (d *Driver) hookCall(ctx context.Context, ts oauth2.TokenSource, method, p string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	resp, err := d.sendURL(ctx, ts, method, d.base+p, rd, body != nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusForbidden:
		return fmt.Errorf("%w: GitHub answered %d to %s %s; managing a repo's webhooks needs admin rights on it, or a fine-grained token's webhooks permission",
			mounts.ErrPermissionDenied, resp.StatusCode, method, p)
	case http.StatusUnprocessableEntity:
		var e struct {
			Message string `json:"message"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		msg := e.Message
		for _, x := range e.Errors {
			msg += "; " + x.Message
		}
		return fmt.Errorf("%w: GitHub refused the webhook: %s", mounts.ErrInvalidArgument, msg)
	}
	if err := status(resp, method != http.MethodGet); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decoding %s %s: %v", mounts.ErrUnavailable, method, p, err)
	}
	return nil
}

var _ mounts.WebhookManager = (*Driver)(nil)
