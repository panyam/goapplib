package githubtest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitHub's lifetimes for a GitHub App's user tokens: eight hours for the access token and six
// months for the refresh token. The fake reports them and enforces neither. ExpireTokens ends
// access tokens instead.
const (
	userTokenTTL    = 8 * time.Hour
	refreshTokenTTL = 184 * 24 * time.Hour
)

// User is a person who can sign in to the fake. AddUser makes one.
type User struct {
	s     *Server
	Login string
	ID    int64

	// Name and Email are what /user answers. AddUser sets Name to Login and leaves Email empty,
	// which /user answers as null, as GitHub does for a private email.
	Name  string
	Email string

	orgs map[string]bool // lowercase org logins
}

// device is one device-flow sign-in, from its device code until a token is issued for it.
type device struct {
	clientID string
	userCode string
	user     *User // set by Approve
	interval time.Duration
	lastPoll time.Time
}

// authCode is one authorization code from the web flow, good for one exchange.
type authCode struct {
	clientID  string
	redirect  string
	challenge string
	user      *User
}

// refreshGrant is what a refresh token stands for.
type refreshGrant struct {
	clientID string
	user     *User
}

// AddUser makes a person who can sign in, a member of each of orgs. Each sign-in issues the
// person a new access token and refresh token, and the fake accepts the access token as AddToken
// does, until ExpireTokens or a refresh retires it. Repo.GrantUser and Repo.HookManagerUsers name
// the person, so a grant holds across refreshes.
func (s *Server) AddUser(login string, orgs ...string) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userSeq++
	u := &User{s: s, Login: login, ID: s.userSeq, Name: login, orgs: map[string]bool{}}
	for _, o := range orgs {
		u.orgs[strings.ToLower(o)] = true
	}
	s.users[strings.ToLower(login)] = u
	return u
}

// SignInAs makes the web flow's authorize step sign in as login, as if that person were signed
// in to GitHub in the browser and approved the app. Until a test calls it, authorize redirects
// back with error=access_denied. It panics on a login AddUser didn't make.
func (s *Server) SignInAs(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[strings.ToLower(login)]
	if u == nil {
		panic("githubtest: SignInAs: no user " + login)
	}
	s.signedIn = u
}

// Approve stands in for the person typing userCode at the verification URL and approving the
// app. The next poll for that device code gets a token for u. It reports false for a user code
// the fake didn't issue or one already used.
func (u *User) Approve(userCode string) bool {
	u.s.mu.Lock()
	defer u.s.mu.Unlock()
	for _, d := range u.s.devices {
		if d.userCode == userCode && d.user == nil {
			d.user = u
			return true
		}
	}
	return false
}

// ExpireTokens ends every access token issued to login, so each one gets 401 Bad credentials, as
// GitHub answers an expired token. The person's refresh tokens still work, so a test can watch the
// app refresh. A token given to AddToken or Grant is unaffected.
func (s *Server) ExpireTokens(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok, u := range s.access {
		if strings.EqualFold(u.Login, login) {
			delete(s.access, tok)
		}
	}
}

// SetDeviceInterval sets how often a device-flow client may poll, for device codes issued after
// the call. GitHub's interval is five seconds. The fake's default is one second, so a test that
// waits through DeviceAccessToken runs quickly.
func (s *Server) SetDeviceInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceInterval = d
}

// userOf returns the person a sign-in token belongs to, or nil. Callers hold s.mu.
func (s *Server) userOf(tok string) *User {
	if tok == "" {
		return nil
	}
	return s.access[tok]
}

// serveSignIn serves /login/oauth/authorize, /login/oauth/access_token and /login/device/code.
func (s *Server) serveSignIn(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.URL.Path == "/login/oauth/authorize" && req.Method == http.MethodGet:
		s.authorize(w, req)
	case req.URL.Path == "/login/oauth/access_token" && req.Method == http.MethodPost:
		s.accessToken(w, req)
	case req.URL.Path == "/login/device/code" && req.Method == http.MethodPost:
		s.deviceCode(w, req)
	default:
		notFound(w)
	}
}

// authorize redirects to redirect_uri with a code for the person SignInAs picked, or with
// error=access_denied when no one is signed in. An unknown client gets a 404 page, since GitHub
// shows an error page rather than redirect to an unchecked URI.
func (s *Server) authorize(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	s.mu.Lock()
	_, known := s.apps[q.Get("client_id")]
	u := s.signedIn
	s.mu.Unlock()
	if !known || err != nil || !redirect.IsAbs() {
		notFound(w)
		return
	}
	if m := q.Get("code_challenge_method"); q.Get("code_challenge") != "" && m != "S256" {
		http.Error(w, "githubtest: code_challenge_method must be S256", http.StatusBadRequest)
		return
	}
	back := redirect.Query()
	back.Set("state", q.Get("state"))
	if u == nil {
		back.Set("error", "access_denied")
	} else {
		code := randHex()
		s.mu.Lock()
		s.codes[code] = authCode{clientID: q.Get("client_id"), redirect: q.Get("redirect_uri"),
			challenge: q.Get("code_challenge"), user: u}
		s.mu.Unlock()
		back.Set("code", code)
	}
	redirect.RawQuery = back.Encode()
	http.Redirect(w, req, redirect.String(), http.StatusFound)
}

// accessToken serves the token endpoint's three grants. Every OAuth error is a 200 with an
// "error" field, as GitHub answers, and x/oauth2 reads it as a *RetrieveError.
func (s *Server) accessToken(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		oauthError(w, req, "bad_request", err.Error())
		return
	}
	clientID, secret := req.PostForm.Get("client_id"), req.PostForm.Get("client_secret")
	if id, sec, ok := req.BasicAuth(); ok {
		clientID, secret = id, sec
	}
	grant := req.PostForm.Get("grant_type")
	if grant == "" {
		grant = "authorization_code"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	app := s.apps[clientID]
	known := app != nil
	var want string
	if known {
		want = app.secret
	}
	// The device flow runs on a public client, so it needs no secret. When one comes, it must match.
	secretOK := secret == want || grant == "urn:ietf:params:oauth:grant-type:device_code" && secret == ""
	if !known || !secretOK {
		oauthError(w, req, "incorrect_client_credentials", "The client_id and/or client_secret passed are incorrect.")
		return
	}
	switch grant {
	case "authorization_code":
		code := req.PostForm.Get("code")
		c, ok := s.codes[code]
		delete(s.codes, code)
		if !ok || c.clientID != clientID || req.PostForm.Has("redirect_uri") && req.PostForm.Get("redirect_uri") != c.redirect ||
			c.challenge != "" && pkceChallenge(req.PostForm.Get("code_verifier")) != c.challenge {
			oauthError(w, req, "bad_verification_code", "The code passed is incorrect or expired.")
			return
		}
		s.issueLocked(w, req, clientID, c.user)
	case "refresh_token":
		rt := req.PostForm.Get("refresh_token")
		g, ok := s.refresh[rt]
		if !ok || g.clientID != clientID {
			oauthError(w, req, "bad_refresh_token", "The refresh token passed is incorrect or expired.")
			return
		}
		delete(s.refresh, rt)
		for tok, r := range s.issuedBy {
			if r == rt {
				delete(s.access, tok)
				delete(s.issuedBy, tok)
				delete(s.accessApp, tok)
			}
		}
		s.issueLocked(w, req, clientID, g.user)
	case "urn:ietf:params:oauth:grant-type:device_code":
		dc := req.PostForm.Get("device_code")
		d, ok := s.devices[dc]
		switch {
		case !ok || d.clientID != clientID:
			oauthError(w, req, "incorrect_device_code", "The device_code provided is not valid.")
		case d.user == nil && time.Now().Sub(d.lastPoll) < d.interval-pollSlack:
			// GitHub adds five seconds to the interval on each slow_down, and a client must
			// keep to the new one.
			d.interval += 5 * time.Second
			d.lastPoll = time.Now()
			oauthError(w, req, "slow_down", "Too many requests have been made in the same timeframe.")
		case d.user == nil:
			d.lastPoll = time.Now()
			oauthError(w, req, "authorization_pending", "The authorization request is still pending.")
		default:
			delete(s.devices, dc)
			s.issueLocked(w, req, clientID, d.user)
		}
	default:
		oauthError(w, req, "unsupported_grant_type", "The grant type "+grant+" is not supported.")
	}
}

// pollSlack forgives a device poll that arrives a little early, since a client's ticker can fire
// a few milliseconds short of the interval.
const pollSlack = 100 * time.Millisecond

// issueLocked mints an access token and refresh token for u and writes them. Callers hold s.mu.
func (s *Server) issueLocked(w http.ResponseWriter, req *http.Request, clientID string, u *User) {
	access, rt := "ghu_"+randHex(), "ghr_"+randHex()
	s.access[access] = u
	s.accessApp[access] = clientID
	s.issuedBy[access] = rt
	s.refresh[rt] = refreshGrant{clientID: clientID, user: u}
	oauthAnswer(w, req, url.Values{
		"access_token":             {access},
		"expires_in":               {strconv.Itoa(int(userTokenTTL.Seconds()))},
		"refresh_token":            {rt},
		"refresh_token_expires_in": {strconv.Itoa(int(refreshTokenTTL.Seconds()))},
		"scope":                    {""},
		"token_type":               {"bearer"},
	})
}

// deviceCode starts a device-flow sign-in. GitHub answers 200 with incorrect_client_credentials
// for an unknown client here too.
func (s *Server) deviceCode(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		oauthError(w, req, "bad_request", err.Error())
		return
	}
	clientID := req.PostForm.Get("client_id")
	if id, _, ok := req.BasicAuth(); ok {
		clientID = id
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apps[clientID]; !ok {
		oauthError(w, req, "incorrect_client_credentials", "The client_id and/or client_secret passed are incorrect.")
		return
	}
	dc := randHex()
	uc := strings.ToUpper(randHex()[:4] + "-" + randHex()[:4])
	s.devices[dc] = &device{clientID: clientID, userCode: uc, interval: s.deviceInterval}
	oauthAnswer(w, req, url.Values{
		"device_code":      {dc},
		"user_code":        {uc},
		"verification_uri": {s.URL + "/login/device"},
		"expires_in":       {"900"},
		"interval":         {strconv.Itoa(int(s.deviceInterval / time.Second))},
	})
}

// serveUser serves GET /user for a sign-in token. A token with no person behind it, such as one
// given to AddToken, gets 404, since the fake has no account to describe.
func (s *Server) serveUser(w http.ResponseWriter, tok string) {
	s.mu.Lock()
	u := s.userOf(tok)
	s.mu.Unlock()
	switch {
	case tok == "":
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Requires authentication"})
	case u == nil:
		notFound(w)
	default:
		var email any
		if u.Email != "" {
			email = u.Email
		}
		writeJSON(w, http.StatusOK, map[string]any{"login": u.Login, "id": u.ID, "name": u.Name, "email": email, "type": "User"})
	}
}

// serveMembership serves GET /orgs/{org}/members/{username}: 204 for a member and 404 otherwise,
// for any accepted token. A token through a modelled App without "members" gets 403.
func (s *Server) serveMembership(w http.ResponseWriter, org, login, tok string, accepted bool) {
	if !accepted {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Requires authentication"})
		return
	}
	if !s.permitted(tok, "members", "read") {
		notAccessible(w)
		return
	}
	s.mu.Lock()
	u := s.users[strings.ToLower(login)]
	member := u != nil && u.orgs[strings.ToLower(org)]
	s.mu.Unlock()
	if !member {
		notFound(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// oauthAnswer writes v as JSON when the client accepts it and as a form otherwise, as GitHub does.
func oauthAnswer(w http.ResponseWriter, req *http.Request, v url.Values) {
	if strings.Contains(req.Header.Get("Accept"), "json") {
		m := map[string]any{}
		for k := range v {
			if n, err := strconv.Atoi(v.Get(k)); err == nil && k != "access_token" && k != "refresh_token" {
				m[k] = n
			} else {
				m[k] = v.Get(k)
			}
		}
		writeJSON(w, http.StatusOK, m)
		return
	}
	w.Header().Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	fmt.Fprint(w, v.Encode())
}

func oauthError(w http.ResponseWriter, req *http.Request, code, desc string) {
	oauthAnswer(w, req, url.Values{"error": {code}, "error_description": {desc},
		"error_uri": {"https://docs.github.com/apps/oauth"}})
}

// pkceChallenge is the S256 challenge for verifier, per RFC 7636.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
