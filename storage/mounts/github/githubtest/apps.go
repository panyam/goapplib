package githubtest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// installationTokenTTL is GitHub's lifetime for an installation token.
const installationTokenTTL = time.Hour

// App is a GitHub App registered on the fake. AddApp makes one.
//
// An App starts unmodelled. Its sign-in tokens then read what the person can read, and the
// membership check answers them, as before Apps had permissions here. SetPermissions or Install
// makes it modelled. Then, as on GitHub:
//
//   - a sign-in token through the App reads a private repo only where the App is installed and the
//     person can read it, and gets 404 on any other private repo;
//   - a repo route that reads files needs the "contents" permission, and a write needs "contents"
//     at "write". Without it the API answers 403 "Resource not accessible by integration";
//   - GET /orgs/{org}/members/{login} needs "members" at "read" or above, with the same 403.
//
// A public repo answers any token, installed or not. GitHub always grants "metadata", so the fake
// never checks it. The hooks API checks no permission.
type App struct {
	s *Server

	// ClientID is the App's OAuth client id, which the sign-in endpoints check.
	ClientID string

	// ID is the App's numeric id. An App JWT names it, or ClientID, as its iss.
	ID int64

	secret   string
	key      *rsa.PrivateKey // made on first use, since a 2048-bit key takes a while
	perms    map[string]string
	installs []*Installation
	hook     *appHook
}

// Installation is an App installed on one account, on all its repos or on a chosen list.
type Installation struct {
	app *App

	// ID names the installation in /app/installations/{id}/access_tokens and in webhooks.
	ID int64

	// Account is the user or org login the App is installed on.
	Account string

	repos map[string]bool // lowercase repo names; empty means every repo of Account
}

type appHook struct {
	url    string
	secret string
	events []string
}

// installToken is a live installation token.
type installToken struct {
	inst    *Installation
	expires time.Time
}

// AddApp registers a GitHub App's OAuth client and returns the App. The sign-in endpoints answer
// only a registered client, and give any other client id, or a wrong secret, GitHub's
// incorrect_client_credentials error. Registering a client id again replaces its secret and keeps
// the App.
func (s *Server) AddApp(clientID, clientSecret string) *App {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a := s.apps[clientID]; a != nil {
		a.secret = clientSecret
		return a
	}
	s.appSeq++
	a := &App{s: s, ClientID: clientID, ID: s.appSeq, secret: clientSecret}
	s.apps[clientID] = a
	return a
}

// SetPermissions sets the App's permissions, such as {"contents": "read", "members": "read"}.
// Levels are "read", "write" and "admin", each including the ones before. It makes the App
// modelled.
func (a *App) SetPermissions(perms map[string]string) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.perms = map[string]string{}
	for k, v := range perms {
		a.perms[k] = v
	}
}

// Install installs the App on account, on each of repos (names without the owner), or on every
// repo of account when repos is empty, including repos added later. It makes the App modelled and
// returns the new installation.
func (a *App) Install(account string, repos ...string) *Installation {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if a.perms == nil {
		a.perms = map[string]string{}
	}
	a.s.installSeq++
	in := &Installation{app: a, ID: a.s.installSeq, Account: account, repos: map[string]bool{}}
	for _, r := range repos {
		in.repos[strings.ToLower(r)] = true
	}
	a.installs = append(a.installs, in)
	return in
}

// SetWebhook sets the App's webhook. Repo.Push delivers a push to it, signed with secret, when the
// App is installed on the repo and events holds "push" or "*". The body carries the installation,
// as an App's webhook does.
func (a *App) SetWebhook(url, secret string, events ...string) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.hook = &appHook{url: url, secret: secret, events: slices.Clone(events)}
}

// SetKey sets the App's private key, which App JWTs must be signed with. Without it, the fake
// makes a 2048-bit key the first time it needs one.
func (a *App) SetKey(k *rsa.PrivateKey) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.key = k
}

// KeyPEM returns the App's private key as PKCS#1 PEM ("RSA PRIVATE KEY"), as GitHub hands it out.
func (a *App) KeyPEM() []byte {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(a.keyLocked())})
}

// Installations returns the App's installations in the order Install made them.
func (a *App) Installations() []*Installation {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	return slices.Clone(a.installs)
}

// Repos returns the names of the repos the installation lists, sorted, or nil when it covers every
// repo of its account.
func (in *Installation) Repos() []string {
	in.app.s.mu.Lock()
	defer in.app.s.mu.Unlock()
	if len(in.repos) == 0 {
		return nil
	}
	return sortedKeys(in.repos)
}

// PushRequest builds the signed push event the App's webhook gets for branch of r, as
// Repo.PushRequest does, with the installation in the body.
func (in *Installation) PushRequest(r *Repo, target string, secret []byte, branch string) *http.Request {
	return SignedRequest(target, secret, "push", r.pushBody(branch, in))
}

// keyLocked returns the App's key, making one if it has none. Callers hold s.mu.
func (a *App) keyLocked() *rsa.PrivateKey {
	if a.key == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic("githubtest: generate App key: " + err.Error())
		}
		a.key = k
	}
	return a.key
}

// modelled reports whether the App has permissions or installations. Callers hold s.mu.
func (a *App) modelled() bool { return a != nil && a.perms != nil }

// covers reports whether the installation includes r. Callers hold s.mu.
func (in *Installation) covers(r *Repo) bool {
	return strings.EqualFold(in.Account, r.Owner) && (len(in.repos) == 0 || in.repos[strings.ToLower(r.Name)])
}

// installedOn reports whether any of the App's installations includes r. Callers hold s.mu.
func (a *App) installedOn(r *Repo) bool {
	return slices.ContainsFunc(a.installs, func(in *Installation) bool { return in.covers(r) })
}

var levels = map[string]int{"read": 1, "write": 2, "admin": 3}

// tokenApp returns the modelled App behind tok, a sign-in token through it or one of its
// installation tokens, or nil. Callers hold s.mu.
func (s *Server) tokenApp(tok string) *App {
	if it, ok := s.installTokens[tok]; ok {
		return it.inst.app
	}
	if s.access[tok] != nil {
		if a := s.apps[s.accessApp[tok]]; a.modelled() {
			return a
		}
	}
	return nil
}

// permitted reports whether tok may use perm at level. A token with no modelled App behind it may
// use anything.
func (s *Server) permitted(tok, perm, level string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.tokenApp(tok)
	return a == nil || levels[a.perms[perm]] >= levels[level]
}

// notAccessible writes GitHub's answer to an App token that lacks a permission.
func notAccessible(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{"message": "Resource not accessible by integration"})
}

// liveInstallToken reports whether tok is an unexpired installation token. Callers hold s.mu.
func (s *Server) liveInstallToken(tok string) bool {
	it, ok := s.installTokens[tok]
	return ok && time.Now().Before(it.expires)
}

// installationAccessToken serves POST /app/installations/{id}/access_tokens. It checks the App JWT
// as GitHub does: RS256, signed with the App's key, iss the App's id or client id, and exp at most
// ten minutes after iat. The token reads the installation's repos within the App's permissions.
// The request body's narrowing of repositories and permissions is ignored.
func (s *Server) installationAccessToken(w http.ResponseWriter, req *http.Request, idPart string) {
	raw, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "A JSON web token could not be decoded"})
		return
	}
	unverified := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(raw, unverified); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "A JSON web token could not be decoded"})
		return
	}
	iss := fmt.Sprint(unverified["iss"])
	s.mu.Lock()
	var app *App
	for _, a := range s.apps {
		if iss == a.ClientID || iss == strconv.FormatInt(a.ID, 10) {
			app = a
		}
	}
	var key *rsa.PrivateKey
	if app != nil {
		key = app.keyLocked()
	}
	s.mu.Unlock()
	if app == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Integration not found"})
		return
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(60*time.Second))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "A JSON web token could not be decoded"})
		return
	}
	iat, _ := claims.GetIssuedAt()
	exp, _ := claims.GetExpirationTime()
	if iat == nil || exp.Sub(iat.Time) > 10*time.Minute {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "'Expiration time' claim ('exp') is too far in the future"})
		return
	}
	id, _ := strconv.ParseInt(idPart, 10, 64)
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(app.installs, func(in *Installation) bool { return in.ID == id })
	if i < 0 {
		notFound(w)
		return
	}
	tok := "ghs_" + randHex()
	expires := time.Now().Add(installationTokenTTL).UTC().Truncate(time.Second)
	s.installTokens[tok] = installToken{inst: app.installs[i], expires: expires}
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "expires_at": expires.Format(time.RFC3339),
		"permissions": app.perms, "repository_selection": selection(app.installs[i])})
}

func selection(in *Installation) string {
	if len(in.repos) == 0 {
		return "all"
	}
	return "selected"
}

// installationRepos serves GET /installation/repositories for an installation token.
func (s *Server) installationRepos(w http.ResponseWriter, tok string) {
	s.mu.Lock()
	it, ok := s.installTokens[tok]
	var out []map[string]any
	if ok {
		for _, k := range sortedKeys(s.repos) {
			r := s.repos[k]
			if it.inst.covers(r) {
				out = append(out, map[string]any{"name": r.Name, "full_name": r.Owner + "/" + r.Name,
					"private": r.Private, "owner": map[string]any{"login": r.Owner}})
			}
		}
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "This endpoint requires an installation token"})
		return
	}
	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"total_count": len(out), "repositories": out})
}

// appDelivery is one App webhook a push goes to, and the installation it is delivered for.
type appDelivery struct {
	hook appHook
	inst *Installation
}

// appDeliveries returns each App webhook that wants r's push events.
func (r *Repo) appDeliveries() []appDelivery {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []appDelivery
	for _, k := range sortedKeys(r.s.apps) {
		a := r.s.apps[k]
		if a.hook == nil || !slices.Contains(a.hook.events, "push") && !slices.Contains(a.hook.events, "*") {
			continue
		}
		for _, in := range a.installs {
			if in.covers(r) {
				out = append(out, appDelivery{*a.hook, in})
				break
			}
		}
	}
	return out
}

// installationJSON is the installation object an App's webhook body carries.
func installationJSON(in *Installation) map[string]any {
	return map[string]any{"id": in.ID, "node_id": fmt.Sprintf("MDIzOkludGVncmF0aW9uSW5zdGFsbGF0aW9u%d", in.ID)}
}
