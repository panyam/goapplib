package gcs_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panyam/goapplib/storage/drivers/gcs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
)

// finalize is the push Pub/Sub sends for a GCS OBJECT_FINALIZE notification, trimmed from
// Google's docs.
const finalize = `{"message":{"attributes":{"bucketId":"designs","eventType":"OBJECT_FINALIZE",
"notificationConfig":"projects/_/buckets/designs/notificationConfigs/1","objectGeneration":"1732183200000000",
"objectId":"boards/jetson/power notes.md","payloadFormat":"JSON_API_V1"},"data":"e30=","messageId":"m-1",
"publishTime":"2026-11-21T10:00:00Z"},"subscription":"projects/acme/subscriptions/designs-push"}`

type fakeVerifier struct{ want string }

func (f fakeVerifier) VerifyPush(_ context.Context, tok string) error {
	if tok != f.want {
		return errors.New("wrong token")
	}
	return nil
}

func push(body, token string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/hooks/gcs", strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func withEvent(eventType string) string {
	return strings.Replace(finalize, "OBJECT_FINALIZE", eventType, 1)
}

func TestParsePush(t *testing.T) {
	v := fakeVerifier{want: "good"}
	for _, typ := range []string{"OBJECT_FINALIZE", "OBJECT_DELETE", "OBJECT_METADATA_UPDATE"} {
		got, err := gcs.ParsePush(push(withEvent(typ), "good"), v)
		if err != nil || len(got) != 1 || got[0].Source != "gs://designs/boards/jetson/power notes.md" {
			t.Errorf("%s: ParsePush = %+v, %v", typ, got, err)
		}
	}
	if got, err := gcs.ParsePush(push(withEvent("OBJECT_ARCHIVE"), "good"), v); err != nil || len(got) != 0 {
		t.Errorf("OBJECT_ARCHIVE: ParsePush = %+v, %v; want nothing", got, err)
	}
	for name, r := range map[string]*http.Request{"no token": push(finalize, ""), "wrong token": push(finalize, "bad")} {
		if got, err := gcs.ParsePush(r, v); !errors.Is(err, gcs.ErrUnauthenticated) {
			t.Errorf("%s: ParsePush = %+v, %v; want ErrUnauthenticated", name, got, err)
		}
	}
	if _, err := gcs.ParsePush(push(finalize, "good"), nil); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ParsePush with no verifier = %v, want ErrInvalidArgument", err)
	}
	if _, err := gcs.ParsePush(push("not json", "good"), v); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ParsePush of junk = %v, want ErrInvalidArgument", err)
	}
}

// googleEnv is a fake Google: a JWKS endpoint for one key, and a way to sign push tokens with it.
type googleEnv struct {
	key  *rsa.PrivateKey
	jwks string
}

func newGoogleEnv(t *testing.T) *googleEnv {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	t.Cleanup(srv.Close)
	return &googleEnv{key: key, jwks: srv.URL}
}

func (g *googleEnv) token(t *testing.T, aud, email string) string {
	t.Helper()
	return g.tokenFrom(t, gcs.GoogleIssuer, aud, email)
}

func (g *googleEnv) tokenFrom(t *testing.T, iss, aud, email string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": iss, "aud": aud, "sub": "1234",
		"email": email, "email_verified": true, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(g.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGoogleVerifier(t *testing.T) {
	g := newGoogleEnv(t)
	const aud, sa = "https://app.example.com/hooks/gcs", "pusher@acme.iam.gserviceaccount.com"
	v := &gcs.GoogleVerifier{Audience: aud, Email: sa, JWKSURL: g.jwks}
	if _, err := gcs.ParsePush(push(finalize, g.token(t, aud, sa)), v); err != nil {
		t.Errorf("ParsePush with a good token = %v", err)
	}
	for name, tok := range map[string]string{
		"other audience":        g.token(t, "https://elsewhere.example.com", sa),
		"other service account": g.token(t, aud, "someone@acme.iam.gserviceaccount.com"),
		"not a jwt":             "abc.def.ghi",
	} {
		if _, err := gcs.ParsePush(push(finalize, tok), v); !errors.Is(err, gcs.ErrUnauthenticated) {
			t.Errorf("%s: ParsePush = %v, want ErrUnauthenticated", name, err)
		}
	}
}

func TestFollowingFindsGCSMountsUnderAPushedObject(t *testing.T) {
	ctx := context.Background()
	srv := fake(t, true)
	put(srv, "boards/jetson/power notes.md", "rev 1")
	put(srv, "other/x.md", "x")
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{newDriver(t, srv)}, mounts.WithCredentials(func(context.Context, string, string) (mounts.Credentials, error) {
		return creds, nil
	}))
	for name, src := range map[string]string{"jetson": "gs://designs/boards/jetson", "other": "gs://designs/other"} {
		if _, err := reg.Create(ctx, mounts.Mount{Name: name, Kind: mounts.KindGCS, Source: src, Mode: mounts.ModeSnapshot, Connection: "gcp"}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	changes, err := gcs.ParsePush(push(finalize, "good"), fakeVerifier{want: "good"})
	if err != nil {
		t.Fatal(err)
	}
	ms, err := reg.Following(ctx, mounts.KindGCS, changes[0])
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.Name)
	}
	if !slices.Equal(got, []string{"jetson"}) {
		t.Errorf("Following = %v, want [jetson]", got)
	}
}

func TestGoogleVerifierAcceptsBothGoogleIssuerForms(t *testing.T) {
	g := newGoogleEnv(t)
	const aud, sa = "https://app.example.com/hooks/gcs", "pusher@acme.iam.gserviceaccount.com"
	v := &gcs.GoogleVerifier{Audience: aud, Email: sa, JWKSURL: g.jwks}
	for _, iss := range []string{"https://accounts.google.com", "accounts.google.com"} {
		if _, err := gcs.ParsePush(push(finalize, g.tokenFrom(t, iss, aud, sa)), v); err != nil {
			t.Errorf("issuer %q: ParsePush = %v, want accepted", iss, err)
		}
	}
	for _, iss := range []string{"https://evil.example.com", "http://accounts.google.com", ""} {
		if _, err := gcs.ParsePush(push(finalize, g.tokenFrom(t, iss, aud, sa)), v); !errors.Is(err, gcs.ErrUnauthenticated) {
			t.Errorf("issuer %q: ParsePush = %v, want ErrUnauthenticated", iss, err)
		}
	}
}
