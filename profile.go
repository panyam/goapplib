package goapplib

import (
	"net/http"
)

// SampleProfilePage provides sample profile page functionality.
// Embed this in your app-specific profile page struct.
type SampleProfilePage[AC any] struct {
	BasePage

	// User information
	UserID            string
	Email             string
	EmailVerified     bool
	Username          string
	Profile           map[string]any
	VerificationSent  bool
	VerificationError string
}

// Load implements Loader[AC] for SampleProfilePage.
func (p *SampleProfilePage[AC]) Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool) {
	p.Title = "Profile"
	p.ActiveTab = "profile"
	p.DisableSplashScreen = true

	if r.URL.Query().Get("verification_sent") == "true" {
		p.VerificationSent = true
	}
	if verifyErr := r.URL.Query().Get("verification_error"); verifyErr != "" {
		p.VerificationError = verifyErr
	}

	return nil, false
}
