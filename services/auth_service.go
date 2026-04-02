package services

import (
	oa "github.com/panyam/oneauth/core"
	la "github.com/panyam/oneauth/localauth"
	"github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

// AuthService orchestrates authentication by coordinating oneauth stores.
//
// Deprecated: AuthService is a thin wrapper around oneauth. For new code,
// use oneauth directly with NewEnsureAuthUserFunc and the helper functions.
// This wrapper is maintained for backwards compatibility.
//
// # Migration Guide
//
// Instead of:
//
//	authService := services.NewAuthService("/path")
//	user, _ := authService.EnsureAuthUser("oauth", "google", token, userInfo)
//
// Use oneauth directly:
//
//	config := oneauth.EnsureAuthUserConfig{
//	    UserStore:     gorm.NewUserStore(db),
//	    IdentityStore: gorm.NewIdentityStore(db),
//	    ChannelStore:  gorm.NewChannelStore(db),
//	    UsernameStore: gorm.NewUsernameStore(db), // Optional
//	}
//	ensureUser := oneauth.NewEnsureAuthUserFunc(config)
//	user, _ := ensureUser("oauth", "google", token, userInfo)
//
// # Integration with UsersService
//
// AuthService manages the auth-level user (oneauth.User), while UsersService
// manages the application-level user profile (goapplib.v1.User). After successful
// authentication, use UsersService.EnsureUser to sync the profile:
//
//	user, err := authService.EnsureAuthUser("oauth", "google", token, userInfo)
//	profile, err := userService.EnsureUser(ctx, user.Id(), name, email, imageUrl)
type AuthService struct {
	UserStore     oa.UserStore
	IdentityStore oa.IdentityStore
	ChannelStore  oa.ChannelStore
	TokenStore    oa.TokenStore
	UsernameStore oa.UsernameStore // Optional - for username uniqueness

	// Internal: cached ensureUser function
	ensureUser func(authtype string, provider string, token any, userInfo map[string]any) (oa.User, error)
}

// NewAuthService creates a new AuthService with file-based stores.
//
// Deprecated: Use oneauth stores directly. See AuthService documentation.
func NewAuthService(storagePath string) *AuthService {
	service := &AuthService{
		UserStore:     fs.NewFSUserStore(storagePath),
		IdentityStore: fs.NewFSIdentityStore(storagePath),
		ChannelStore:  fs.NewFSChannelStore(storagePath),
		TokenStore:    fs.NewFSTokenStore(storagePath),
	}
	service.initEnsureUser()
	return service
}

// NewAuthServiceWithStores creates a new AuthService with custom stores.
//
// Deprecated: Use oneauth stores directly. See AuthService documentation.
func NewAuthServiceWithStores(userStore oa.UserStore, identityStore oa.IdentityStore, channelStore oa.ChannelStore, tokenStore oa.TokenStore) *AuthService {
	service := &AuthService{
		UserStore:     userStore,
		IdentityStore: identityStore,
		ChannelStore:  channelStore,
		TokenStore:    tokenStore,
	}
	service.initEnsureUser()
	return service
}

// NewAuthServiceWithAllStores creates a new AuthService with all stores including UsernameStore.
func NewAuthServiceWithAllStores(userStore oa.UserStore, identityStore oa.IdentityStore, channelStore oa.ChannelStore, tokenStore oa.TokenStore, usernameStore oa.UsernameStore) *AuthService {
	service := &AuthService{
		UserStore:     userStore,
		IdentityStore: identityStore,
		ChannelStore:  channelStore,
		TokenStore:    tokenStore,
		UsernameStore: usernameStore,
	}
	service.initEnsureUser()
	return service
}

// initEnsureUser initializes the internal ensureUser function using oneauth
func (s *AuthService) initEnsureUser() {
	config := la.EnsureAuthUserConfig{
		UserStore:     s.UserStore,
		IdentityStore: s.IdentityStore,
		ChannelStore:  s.ChannelStore,
		UsernameStore: s.UsernameStore,
	}
	s.ensureUser = la.NewEnsureAuthUserFunc(config)
}

// CreateLocalUser creates a new user with local authentication.
// Pass-through to oneauth.NewCreateUserFunc.
func (s *AuthService) CreateLocalUser(creds *oa.Credentials) (oa.User, error) {
	return la.NewCreateUserFunc(s.UserStore, s.IdentityStore, s.ChannelStore)(creds)
}

// ValidateLocalCredentials validates username/password and returns the user.
// Pass-through to oneauth.NewCredentialsValidator.
func (s *AuthService) ValidateLocalCredentials(username, password, usernameType string) (oa.User, error) {
	return la.NewCredentialsValidator(s.IdentityStore, s.ChannelStore, s.UserStore)(username, password, usernameType)
}

// ValidateLocalCredentialsWithUsername validates credentials allowing username-based login.
// Pass-through to oneauth.NewCredentialsValidatorWithUsername.
func (s *AuthService) ValidateLocalCredentialsWithUsername(username, password, usernameType string) (oa.User, error) {
	if s.UsernameStore == nil {
		// Fall back to standard validator if no UsernameStore
		return s.ValidateLocalCredentials(username, password, usernameType)
	}
	return la.NewCredentialsValidatorWithUsername(s.IdentityStore, s.ChannelStore, s.UserStore, s.UsernameStore)(username, password, usernameType)
}

// VerifyEmailByToken verifies an email using a verification token.
// Pass-through to oneauth.NewVerifyEmailFunc.
func (s *AuthService) VerifyEmailByToken(token string) error {
	return la.NewVerifyEmailFunc(s.IdentityStore, s.TokenStore)(token)
}

// UpdatePassword updates the password for a user identified by email.
// Pass-through to oneauth.NewUpdatePasswordFunc.
func (s *AuthService) UpdatePassword(email, newPassword string) error {
	return la.NewUpdatePasswordFunc(s.IdentityStore, s.ChannelStore)(email, newPassword)
}

// EnsureAuthUser handles user creation/lookup for both OAuth and local authentication.
// Pass-through to oneauth.NewEnsureAuthUserFunc with channel linking support.
func (s *AuthService) EnsureAuthUser(authtype, provider string, token *oauth2.Token, userInfo map[string]any) (oa.User, error) {
	return s.ensureUser(authtype, provider, token, userInfo)
}

// --- Store interface pass-throughs (for interface compliance) ---

// Implement oa.UserStore interface
func (s *AuthService) CreateUser(userId string, isActive bool, profile map[string]any) (oa.User, error) {
	return s.UserStore.CreateUser(userId, isActive, profile)
}

func (s *AuthService) GetUserById(userId string) (oa.User, error) {
	return s.UserStore.GetUserById(userId)
}

func (s *AuthService) SaveUser(user oa.User) error {
	return s.UserStore.SaveUser(user)
}

// Implement oa.IdentityStore interface
func (s *AuthService) GetIdentity(identityType, identityValue string, createIfMissing bool) (*oa.Identity, bool, error) {
	return s.IdentityStore.GetIdentity(identityType, identityValue, createIfMissing)
}

func (s *AuthService) SaveIdentity(identity *oa.Identity) error {
	return s.IdentityStore.SaveIdentity(identity)
}

func (s *AuthService) SetUserForIdentity(identityType, identityValue string, newUserId string) error {
	return s.IdentityStore.SetUserForIdentity(identityType, identityValue, newUserId)
}

func (s *AuthService) MarkIdentityVerified(identityType, identityValue string) error {
	return s.IdentityStore.MarkIdentityVerified(identityType, identityValue)
}

func (s *AuthService) GetUserIdentities(userId string) ([]*oa.Identity, error) {
	return s.IdentityStore.GetUserIdentities(userId)
}

// Implement oa.ChannelStore interface
func (s *AuthService) GetChannel(provider string, identityKey string, createIfMissing bool) (*oa.Channel, bool, error) {
	return s.ChannelStore.GetChannel(provider, identityKey, createIfMissing)
}

func (s *AuthService) SaveChannel(channel *oa.Channel) error {
	return s.ChannelStore.SaveChannel(channel)
}

func (s *AuthService) GetChannelsByIdentity(identityKey string) ([]*oa.Channel, error) {
	return s.ChannelStore.GetChannelsByIdentity(identityKey)
}
