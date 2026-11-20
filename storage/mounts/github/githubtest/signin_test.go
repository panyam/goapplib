package githubtest_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	oneauth "github.com/panyam/oneauth/oauth2"
	"golang.org/x/oauth2"
)

const (
	clientID     = "Iv1.fake"
	clientSecret = "s3cret"
	callbackURL  = "http://app.test/auth/github/callback/"
)

// noRedirects is a client that hands back each redirect instead of following it.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func get(t *testing.T, s *githubtest.Server, path, tok string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, s.URL+path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// signIn runs oneauth's GitHub sign-in against s, the way a browser would, and returns the token
// and profile the app's HandleUser got.
func signIn(t *testing.T, s *githubtest.Server) (*oauth2.Token, map[string]any, *oneauth.GithubOAuth2) {
	t.Helper()
	var tok *oauth2.Token
	var info map[string]any
	p := oneauth.NewGithubOAuth2(clientID, clientSecret, callbackURL,
		func(_, _ string, t *oauth2.Token, i map[string]any, _ http.ResponseWriter, _ *http.Request) {
			tok, info = t, i
		})
	if err := p.SetEnterpriseURL(s.URL); err != nil {
		t.Fatal(err)
	}

	// The app's login page sets its state and PKCE cookies and sends the browser to GitHub.
	login := httptest.NewRecorder()
	p.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/", nil))
	resp, err := noRedirects.Get(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(back.String(), callbackURL) {
		t.Fatalf("authorize answered %d to %q, want a redirect to the callback", resp.StatusCode, resp.Header.Get("Location"))
	}

	// GitHub sends the browser back to the app's callback with the cookies the login page set.
	cb := httptest.NewRequest(http.MethodGet, "/callback/?"+back.RawQuery, nil)
	for _, c := range login.Result().Cookies() {
		cb.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	p.Handler().ServeHTTP(rr, cb)
	if tok == nil {
		t.Fatalf("HandleUser never ran; callback answered %d %s", rr.Code, rr.Body.String())
	}
	return tok, info, p
}

func TestSignInThroughOneauthReadsAGrantedRepoAndRefreshes(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	s.AddUser("octo", "acme")
	s.AddUser("other")
	secret := s.AddRepo("acme", "secret", true, map[string]string{"a.md": "hi"})
	secret.GrantUser("octo")
	s.SignInAs("octo")

	tok, info, p := signIn(t, s)
	if info["login"] != "octo" {
		t.Errorf("HandleUser got login %v, want octo", info["login"])
	}
	if tok.RefreshToken == "" || tok.Expiry.IsZero() {
		t.Errorf("token %+v has no refresh token or expiry, as a GitHub App user token has", tok)
	}
	if code, body := get(t, s, "/repos/acme/secret", tok.AccessToken); code != http.StatusOK {
		t.Fatalf("granted user reads the private repo: %d %s", code, body)
	}

	s.ExpireTokens("octo")
	if code, _ := get(t, s, "/api/v3/user", tok.AccessToken); code != http.StatusUnauthorized {
		t.Errorf("expired token on /user: %d, want 401", code)
	}

	// The app refreshes as x/oauth2 does once the token has expired.
	tok.Expiry = time.Now().Add(-time.Minute)
	old := *tok
	cfg := oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: p.OAuthEndpoint()}
	fresh, err := cfg.TokenSource(context.Background(), tok).Token()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AccessToken == old.AccessToken || fresh.RefreshToken == old.RefreshToken {
		t.Errorf("refresh kept the old access or refresh token")
	}
	if code, body := get(t, s, "/repos/acme/secret", fresh.AccessToken); code != http.StatusOK {
		t.Errorf("refreshed token reads the granted repo: %d %s", code, body)
	}
	if code, body := get(t, s, "/api/v3/user", fresh.AccessToken); code != http.StatusOK || !strings.Contains(body, `"login":"octo"`) {
		t.Errorf("/api/v3/user with the refreshed token: %d %s", code, body)
	}

	// A used refresh token is gone, as on GitHub.
	_, err = cfg.TokenSource(context.Background(), &old).Token()
	if re := (*oauth2.RetrieveError)(nil); !errors.As(err, &re) || re.ErrorCode != "bad_refresh_token" {
		t.Errorf("reusing a refresh token: %v, want bad_refresh_token", err)
	}

	// Another person, signed in the same way, can't see the repo.
	s.SignInAs("other")
	otherTok, _, _ := signIn(t, s)
	if code, _ := get(t, s, "/repos/acme/secret", otherTok.AccessToken); code != http.StatusNotFound {
		t.Errorf("ungranted user reads the private repo: %d, want 404", code)
	}
}

func TestRefreshRetiresTheOldAccessToken(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	s.AddUser("octo")
	s.SignInAs("octo")
	tok, _, p := signIn(t, s)

	cfg := oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: p.OAuthEndpoint()}
	stale := *tok
	stale.Expiry = time.Now().Add(-time.Minute)
	if _, err := cfg.TokenSource(context.Background(), &stale).Token(); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, s, "/user", tok.AccessToken); code != http.StatusUnauthorized {
		t.Errorf("access token after its refresh: %d, want 401", code)
	}
}

func TestDeviceFlow(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	octo := s.AddUser("octo")
	cfg := oauth2.Config{ClientID: clientID, Endpoint: oauth2.Endpoint{
		DeviceAuthURL: s.URL + "/login/device/code", TokenURL: s.URL + "/login/oauth/access_token"}}
	ctx := context.Background()

	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if da.UserCode == "" || da.VerificationURI == "" || da.Interval != 1 {
		t.Fatalf("device auth %+v, want a user code, a verification URI and interval 1", da)
	}
	if got := poll(t, s, da.DeviceCode); got != "authorization_pending" {
		t.Errorf("poll before approval: %q, want authorization_pending", got)
	}
	if !octo.Approve(da.UserCode) {
		t.Fatal("Approve refused the user code")
	}
	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, s, "/user", tok.AccessToken); code != http.StatusOK || !strings.Contains(body, `"login":"octo"`) {
		t.Errorf("/user with the device token: %d %s", code, body)
	}
	if octo.Approve(da.UserCode) {
		t.Error("Approve took a user code twice")
	}
}

func TestDevicePollTooSoonSlowsDown(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	cfg := oauth2.Config{ClientID: clientID, Endpoint: oauth2.Endpoint{DeviceAuthURL: s.URL + "/login/device/code"}}
	da, err := cfg.DeviceAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := poll(t, s, da.DeviceCode); got != "authorization_pending" {
		t.Fatalf("first poll: %q", got)
	}
	if got := poll(t, s, da.DeviceCode); got != "slow_down" {
		t.Errorf("second poll at once: %q, want slow_down", got)
	}
	if got := poll(t, s, "nope"); got != "incorrect_device_code" {
		t.Errorf("unknown device code: %q, want incorrect_device_code", got)
	}
}

// poll asks for a device-flow token once and returns the OAuth error code, or "" for a token.
func poll(t *testing.T, s *githubtest.Server, deviceCode string) string {
	t.Helper()
	return tokenCall(t, s, url.Values{"client_id": {clientID}, "device_code": {deviceCode},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}})
}

// tokenCall posts form to the token endpoint, as a client with no Accept header does, and returns
// the OAuth error code, or "" when a token came back. GitHub answers each error with 200.
func tokenCall(t *testing.T, s *githubtest.Server, form url.Values) string {
	t.Helper()
	resp, err := http.PostForm(s.URL+"/login/oauth/access_token", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token endpoint answered %d %s", resp.StatusCode, b)
	}
	v, err := url.ParseQuery(string(b))
	if err != nil {
		t.Fatalf("token endpoint answered %q, not a form", b)
	}
	if v.Get("error") == "" && v.Get("access_token") == "" {
		t.Fatalf("token endpoint answered %q, with neither a token nor an error", b)
	}
	return v.Get("error")
}

// authorizeCode runs the authorize step by hand and returns the code.
func authorizeCode(t *testing.T, s *githubtest.Server, challenge string) string {
	t.Helper()
	q := url.Values{"client_id": {clientID}, "redirect_uri": {callbackURL}, "state": {"st"}}
	if challenge != "" {
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
	}
	resp, err := noRedirects.Get(s.URL + "/login/oauth/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	back, _ := url.Parse(resp.Header.Get("Location"))
	if back.Query().Get("state") != "st" || back.Query().Get("code") == "" {
		t.Fatalf("authorize redirected to %q, want the callback with a code and the state", back)
	}
	return back.Query().Get("code")
}

func TestTokenEndpointRefusals(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	s.AddUser("octo")
	s.SignInAs("octo")
	exchange := func(code, verifier string) url.Values {
		return url.Values{"client_id": {clientID}, "client_secret": {clientSecret}, "code": {code},
			"redirect_uri": {callbackURL}, "code_verifier": {verifier}}
	}
	verifier := oauth2.GenerateVerifier()

	for name, form := range map[string]url.Values{
		"unknown client": {"client_id": {"nope"}, "client_secret": {clientSecret}, "code": {"x"}},
		"wrong secret":   {"client_id": {clientID}, "client_secret": {"wrong"}, "code": {"x"}},
	} {
		if got := tokenCall(t, s, form); got != "incorrect_client_credentials" {
			t.Errorf("%s: %q, want incorrect_client_credentials", name, got)
		}
	}

	code := authorizeCode(t, s, oauth2.S256ChallengeFromVerifier(verifier))
	if got := tokenCall(t, s, exchange(code, "a-different-verifier")); got != "bad_verification_code" {
		t.Errorf("wrong PKCE verifier: %q, want bad_verification_code", got)
	}

	code = authorizeCode(t, s, oauth2.S256ChallengeFromVerifier(verifier))
	if got := tokenCall(t, s, exchange(code, verifier)); got != "" {
		t.Fatalf("right verifier: %q, want a token", got)
	}
	if got := tokenCall(t, s, exchange(code, verifier)); got != "bad_verification_code" {
		t.Errorf("reused code: %q, want bad_verification_code", got)
	}

	form := exchange(authorizeCode(t, s, ""), "")
	form.Set("redirect_uri", "http://elsewhere.test/cb")
	if got := tokenCall(t, s, form); got != "bad_verification_code" {
		t.Errorf("code sent with another redirect_uri: %q, want bad_verification_code", got)
	}
}

func TestAuthorizeWithNoOneSignedInIsDenied(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	resp, err := noRedirects.Get(s.URL + "/login/oauth/authorize?" + url.Values{
		"client_id": {clientID}, "redirect_uri": {callbackURL}, "state": {"st"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	back, _ := url.Parse(resp.Header.Get("Location"))
	if back.Query().Get("error") != "access_denied" || back.Query().Get("code") != "" {
		t.Errorf("authorize redirected to %q, want error=access_denied and no code", back)
	}
}

func TestOrgMembership(t *testing.T) {
	s := githubtest.New(t)
	s.AddUser("octo", "Acme")
	s.AddUser("other")
	s.AddToken("tok")
	for path, want := range map[string]int{
		"/orgs/acme/members/octo":        http.StatusNoContent,
		"/api/v3/orgs/acme/members/octo": http.StatusNoContent,
		"/orgs/acme/members/other":       http.StatusNotFound,
		"/orgs/acme/members/nobody":      http.StatusNotFound,
		"/orgs/other-org/members/octo":   http.StatusNotFound,
	} {
		if code, _ := get(t, s, path, "tok"); code != want {
			t.Errorf("%s: %d, want %d", path, code, want)
		}
	}
	if code, _ := get(t, s, "/orgs/acme/members/octo", ""); code != http.StatusUnauthorized {
		t.Errorf("membership with no token: %d, want 401", code)
	}
}

func TestEnterprisePrefixReachesRepoRoutes(t *testing.T) {
	s := githubtest.New(t)
	s.AddRepo("acme", "pub", false, map[string]string{"a.md": "hi"})
	code, body := get(t, s, "/api/v3/repos/acme/pub", "")
	if code != http.StatusOK || !strings.Contains(body, `"full_name":"acme/pub"`) {
		t.Errorf("/api/v3/repos/acme/pub: %d %s", code, body)
	}
}

func TestHookManagerUsers(t *testing.T) {
	s := githubtest.New(t)
	s.AddApp(clientID, clientSecret)
	s.AddUser("admin")
	s.AddUser("reader")
	r := s.AddRepo("acme", "app", true, nil)
	r.GrantUser("admin", "reader")
	r.HookManagerUsers("admin")

	s.SignInAs("admin")
	admin, _, _ := signIn(t, s)
	s.SignInAs("reader")
	reader, _, _ := signIn(t, s)
	if code, body := get(t, s, "/repos/acme/app/hooks", admin.AccessToken); code != http.StatusOK {
		t.Errorf("hook manager lists hooks: %d %s", code, body)
	}
	if code, _ := get(t, s, "/repos/acme/app/hooks", reader.AccessToken); code != http.StatusNotFound {
		t.Errorf("reader lists hooks: %d, want 404", code)
	}
}
