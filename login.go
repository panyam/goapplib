package goapplib

import (
	"net/http"
)

// LoginConfig controls which login methods are available.
type LoginConfig struct {
	EnableEmailLogin     bool
	EnableGoogleLogin    bool
	EnableGitHubLogin    bool
	EnableMicrosoftLogin bool
	EnableAppleLogin     bool
}

// SampleLoginPage provides sample login page functionality.
// Embed this in your app-specific login page struct.
type SampleLoginPage[AC any] struct {
	BasePage
	CallbackURL string
	CsrfToken   string
	Config      LoginConfig
}

// Load implements Loader[AC] for SampleLoginPage.
func (p *SampleLoginPage[AC]) Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool) {
	p.DisableSplashScreen = true
	p.CallbackURL = r.URL.Query().Get("callbackURL")
	return nil, false
}

// SampleRegisterPage provides sample registration page functionality.
// Embed this in your app-specific register page struct.
type SampleRegisterPage[AC any] struct {
	BasePage
	CallbackURL    string
	CsrfToken      string
	Name           string
	Email          string
	Password       string
	VerifyPassword string
	Errors         map[string]string
}

// Load implements Loader[AC] for SampleRegisterPage.
func (p *SampleRegisterPage[AC]) Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool) {
	p.CallbackURL = r.URL.Query().Get("callbackURL")
	return nil, false
}
