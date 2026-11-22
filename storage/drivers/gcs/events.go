package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/oneauth/client"
	"github.com/panyam/oneauth/keys"
)

// ErrUnauthenticated means a Pub/Sub push carried no bearer token, or one the PushVerifier
// refused. Answer 401 or 403, and Pub/Sub retries with backoff.
var ErrUnauthenticated = errors.New("gcs: Pub/Sub push not authenticated")

// PushVerifier checks the OIDC token Pub/Sub sends with an authenticated push subscription.
// GoogleVerifier is the one to use. A test passes its own.
type PushVerifier interface {
	// VerifyPush returns nil when token is a valid token for this endpoint, and an error saying
	// which check failed otherwise.
	VerifyPush(ctx context.Context, token string) error
}

// Google's issuer and signing keys for the OIDC tokens Pub/Sub sends. Google issues ID tokens
// with either issuer form, and its Pub/Sub docs say to accept both.
const (
	GoogleIssuer     = "https://accounts.google.com"
	GoogleIssuerBare = "accounts.google.com"
	GoogleJWKS       = "https://www.googleapis.com/oauth2/v3/certs"
)

// GoogleVerifier checks a Pub/Sub push token's signature against Google's keys, its issuer
// (either of Google's two forms), its audience and, when Email is set, the service account that signed it. Share one per process,
// since it caches Google's keys. It fetches them on first use, and again when a token names an
// unknown key, so a key rotation needs no restart.
type GoogleVerifier struct {
	// Audience is the audience the push subscription sets, usually the endpoint's URL. Required.
	Audience string

	// Email, when set, is the service account the subscription pushes as. A token for any other
	// account is refused. Leave it empty only when the audience alone is secret enough.
	Email string

	// Issuer, when set, is the one issuer a token must carry. Empty accepts either of Google's
	// forms, GoogleIssuer or GoogleIssuerBare. JWKSURL defaults to GoogleJWKS. A test points them
	// at a fake.
	Issuer  string
	JWKSURL string

	// HTTPClient fetches the keys. Nil means http.DefaultClient.
	HTTPClient *http.Client

	once sync.Once
	keys keys.KeyLookup
}

// VerifyPush checks token as the type comment describes.
func (g *GoogleVerifier) VerifyPush(ctx context.Context, token string) error {
	if g.Audience == "" {
		return fmt.Errorf("%w: GoogleVerifier has no Audience", mounts.ErrInvalidArgument)
	}
	g.once.Do(func() {
		u := g.JWKSURL
		if u == "" {
			u = GoogleJWKS
		}
		var opts []keys.JWKSOption
		if g.HTTPClient != nil {
			opts = append(opts, keys.WithHTTPClient(g.HTTPClient))
		}
		g.keys = keys.NewJWKSKeyStore(u, opts...)
	})
	iss := g.Issuer
	if iss == "" {
		// VerifyIDToken checks one exact issuer, so pick the form the token claims, from the two
		// Google uses. VerifyIDToken then checks the signature and the issuer for real.
		iss = GoogleIssuer
		var c jwt.RegisteredClaims
		if _, _, err := jwt.NewParser().ParseUnverified(token, &c); err == nil && c.Issuer == GoogleIssuerBare {
			iss = GoogleIssuerBare
		}
	}
	claims, err := client.VerifyIDToken(ctx, &client.VerifyIDTokenRequest{IDToken: token, Issuer: iss, ClientID: g.Audience, Keys: g.keys})
	if err != nil {
		return err
	}
	if g.Email != "" && (claims.Email != g.Email || !claims.EmailVerified) {
		return fmt.Errorf("token is for %q, not %q", claims.Email, g.Email)
	}
	return nil
}

// maxPushBody caps what ParsePush reads. A GCS notification is a few KB.
const maxPushBody = 1 << 20

// ParsePush reads a Pub/Sub push carrying a GCS object notification, after v accepts its bearer
// token. It returns one Change for a finalized (created or overwritten) object, a deleted object,
// or changed metadata, whose Source names the object, such as "gs://designs/boards/jetson/README.md".
// Registry.Following then finds every mount whose prefix holds it. Ref and Version are empty, as in
// s3.ParseEvent. Other event types return no change and no error, so the app answers 200 and
// Pub/Sub doesn't retry them. A nil v returns mounts.ErrInvalidArgument, since an unauthenticated
// push endpoint lets anyone make the app re-resolve its mounts.
func ParsePush(r *http.Request, v PushVerifier) ([]mounts.Change, error) {
	if v == nil {
		return nil, fmt.Errorf("%w: ParsePush needs a PushVerifier", mounts.ErrInvalidArgument)
	}
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || tok == "" {
		return nil, fmt.Errorf("%w: no bearer token", ErrUnauthenticated)
	}
	if err := v.VerifyPush(r.Context(), tok); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPushBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPushBody {
		return nil, fmt.Errorf("%w: push body over %d bytes", mounts.ErrInvalidArgument, maxPushBody)
	}
	var push struct {
		Message struct {
			Attributes map[string]string `json:"attributes"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &push); err != nil {
		return nil, fmt.Errorf("%w: not a Pub/Sub push: %v", mounts.ErrInvalidArgument, err)
	}
	a := push.Message.Attributes
	switch a["eventType"] {
	case "OBJECT_FINALIZE", "OBJECT_DELETE", "OBJECT_METADATA_UPDATE":
	default:
		return nil, nil
	}
	if a["bucketId"] == "" || a["objectId"] == "" {
		return nil, fmt.Errorf("%w: notification names bucket %q and object %q", mounts.ErrInvalidArgument, a["bucketId"], a["objectId"])
	}
	return []mounts.Change{{Source: "gs://" + a["bucketId"] + "/" + a["objectId"]}}, nil
}
