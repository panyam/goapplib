package msgraph

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
)

// ErrBadClientState means a notification's clientState isn't the one the app gave
// CreateSubscription, so it didn't come from that subscription. The app answers 202 anyway, as
// Graph asks, and ignores the body.
var ErrBadClientState = errors.New("msgraph: notification clientState doesn't match")

// maxNotification caps a notification body. Graph batches at most a few hundred small entries.
const maxNotification = 1 << 20

// Subscription is a Graph change-notification subscription on a drive.
type Subscription struct {
	ID              string    `json:"id"`
	Resource        string    `json:"resource"`
	NotificationURL string    `json:"notificationUrl"`
	ClientState     string    `json:"clientState"`
	Expires         time.Time `json:"expirationDateTime"`
}

// CreateSubscription asks Graph to POST to notificationURL whenever something in m's drive
// changes. Graph first sends notificationURL a validation request, which the app's handler answers
// with AnswerValidation, so the handler must be serving before this call. clientState is a secret
// the app picks and later passes to ParseNotification. Graph caps a drive subscription at about
// 30 days, and the app renews it with RenewSubscription before expires. Graph watches a whole
// drive, not one folder.
func (d *Driver) CreateSubscription(ctx context.Context, m mounts.Mount, creds mounts.Credentials, notificationURL, clientState string, expires time.Time) (Subscription, error) {
	dr, l, err := d.locate(m.Source, creds)
	if err != nil {
		return Subscription{}, err
	}
	body := map[string]any{
		"changeType":         "updated",
		"notificationUrl":    notificationURL,
		"resource":           l.resource + "/root",
		"expirationDateTime": expires.UTC().Format(time.RFC3339),
		"clientState":        clientState,
	}
	var out Subscription
	err = dr.g.call(ctx, http.MethodPost, "/subscriptions", body, &out)
	return out, err
}

// RenewSubscription moves subscription id's expiry to expires.
func (d *Driver) RenewSubscription(ctx context.Context, creds mounts.Credentials, id string, expires time.Time) (Subscription, error) {
	g, err := d.graph(creds)
	if err != nil {
		return Subscription{}, err
	}
	var out Subscription
	err = g.call(ctx, http.MethodPatch, "/subscriptions/"+url.PathEscape(id), map[string]string{"expirationDateTime": expires.UTC().Format(time.RFC3339)}, &out)
	return out, err
}

// DeleteSubscription ends subscription id. One that is already gone isn't an error.
func (d *Driver) DeleteSubscription(ctx context.Context, creds mounts.Credentials, id string) error {
	g, err := d.graph(creds)
	if err != nil {
		return err
	}
	err = g.call(ctx, http.MethodDelete, "/subscriptions/"+url.PathEscape(id), nil, nil)
	if errors.Is(err, mounts.ErrNotFound) {
		return nil
	}
	return err
}

// AnswerValidation answers Graph's validation request, which carries a validationToken query
// parameter, by echoing the token as text/plain with 200. It reports whether r was one, so a
// handler calls it first and returns when it is true. The plain-text content type keeps a browser
// from running a crafted token as HTML.
func AnswerValidation(w http.ResponseWriter, r *http.Request) bool {
	tok := r.URL.Query().Get("validationToken")
	if tok == "" {
		return false
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, tok)
	return true
}

// ParseNotification reads a Graph change notification and returns one mounts.Change per entry.
// Each Change's Source is the drive root that changed, such as "msgraph://drives/b!x/root", with
// Default set and no Ref, since Graph says only that something in the drive changed. The app then
// calls Delta on its mounts in that drive to learn which files. Every entry's clientState must
// equal clientState, compared in constant time, or ParseNotification returns ErrBadClientState
// and no changes.
func ParseNotification(r *http.Request, clientState string) ([]mounts.Change, error) {
	var body struct {
		Value []struct {
			SubscriptionID string `json:"subscriptionId"`
			ClientState    string `json:"clientState"`
			Resource       string `json:"resource"`
			ChangeType     string `json:"changeType"`
		} `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxNotification)).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: notification body: %v", mounts.ErrInvalidArgument, err)
	}
	var out []mounts.Change
	seen := map[string]bool{}
	for _, v := range body.Value {
		if subtle.ConstantTimeCompare([]byte(v.ClientState), []byte(clientState)) != 1 {
			return nil, ErrBadClientState
		}
		src := Scheme + "://" + strings.Trim(v.Resource, "/")
		if seen[src] {
			continue
		}
		seen[src] = true
		out = append(out, mounts.Change{Source: src, Default: true})
	}
	return out, nil
}

// ContainsSource implements mounts.SourceContainer, so Registry.Following finds the mounts a
// notification is about. Graph notifies per drive, and ParseNotification names the drive root,
// such as "msgraph://drives/<id>/root". So a change contains a mount when the mount's folder is in
// that drive, the reverse of the bucket rule it would otherwise inherit, where a mount contains an
// object under its prefix. A change naming a file, as a caller might build from Delta, still finds
// the mount holding it.
func (d *Driver) ContainsSource(mountSource, changed string) bool {
	in := func(child, parent string) bool { return child == parent || strings.HasPrefix(child, parent+"/") }
	return in(mountSource, changed) || d.Driver.ContainsSource(mountSource, changed)
}

var _ mounts.SourceContainer = (*Driver)(nil)
