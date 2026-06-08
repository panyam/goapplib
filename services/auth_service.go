package services

import (
	"context"

	"github.com/panyam/oneauth/accounts"
	"github.com/panyam/oneauth/federatedauth"
	"github.com/panyam/oneauth/localauth"
	"github.com/panyam/oneauth/stores/fs"
	"golang.org/x/oauth2"
)

// AuthService orchestrates authentication by coordinating oneauth stores.
//
// Deprecated: AuthService is a thin wrapper around oneauth. For new code,
// use oneauth directly with federatedauth.NewEnsureAuthUserFunc and the
// helper functions. This wrapper is maintained for backwards compatibility.
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
//	config := federatedauth.EnsureAuthUserConfig{
//	    UserStore:     gorm.NewUserStore(db),
//	    IdentityStore: gorm.NewIdentityStore(db),
//	    ChannelStore:  gorm.NewChannelStore(db),
//	    UsernameStore: gorm.NewUsernameStore(db), // Optional
//	}
//	ensureUser := federatedauth.NewEnsureAuthUserFunc(config)
//	user, _ := ensureUser("oauth", "google", token, userInfo)
//
// # Integration with UsersService
//
// AuthService manages the auth-level user (accounts.User), while UsersService
// manages the application-level user profile (goapplib.v1.User). After successful
// authentication, use UsersService.EnsureUser to sync the profile:
//
//	user, err := authService.EnsureAuthUser("oauth", "google", token, userInfo)
//	profile, err := userService.EnsureUser(ctx, user.Id(), name, email, imageUrl)
type AuthService struct {
	UserStore     accounts.UserStore
	IdentityStore accounts.IdentityStore
	ChannelStore  accounts.ChannelStore
	TokenStore    localauth.VerificationTokenStore
	UsernameStore accounts.UsernameStore // Optional - for username uniqueness

	// Internal: cached ensureUser function
	ensureUser federatedauth.EnsureAuthUserFunc
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
func NewAuthServiceWithStores(userStore accounts.UserStore, identityStore accounts.IdentityStore, channelStore accounts.ChannelStore, tokenStore localauth.VerificationTokenStore) *AuthService {
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
func NewAuthServiceWithAllStores(userStore accounts.UserStore, identityStore accounts.IdentityStore, channelStore accounts.ChannelStore, tokenStore localauth.VerificationTokenStore, usernameStore accounts.UsernameStore) *AuthService {
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

// initEnsureUser initializes the internal ensureUser function using oneauth.
func (s *AuthService) initEnsureUser() {
	config := federatedauth.EnsureAuthUserConfig{
		UserStore:     s.UserStore,
		IdentityStore: s.IdentityStore,
		ChannelStore:  s.ChannelStore,
		UsernameStore: s.UsernameStore,
	}
	s.ensureUser = federatedauth.NewEnsureAuthUserFunc(config)
}

// CreateLocalUser creates a new user with local authentication.
// Pass-through to localauth.NewCreateUserFunc.
func (s *AuthService) CreateLocalUser(creds *localauth.Credentials) (accounts.User, error) {
	return localauth.NewCreateUserFunc(s.UserStore, s.IdentityStore, s.ChannelStore)(creds)
}

// ValidateLocalCredentials validates username/password and returns the user.
// Pass-through to localauth.NewCredentialsValidator.
func (s *AuthService) ValidateLocalCredentials(username, password, usernameType string) (accounts.User, error) {
	return localauth.NewCredentialsValidator(s.IdentityStore, s.ChannelStore, s.UserStore)(username, password, usernameType)
}

// ValidateLocalCredentialsWithUsername validates credentials allowing username-based login.
// Pass-through to localauth.NewCredentialsValidatorWithUsername.
func (s *AuthService) ValidateLocalCredentialsWithUsername(username, password, usernameType string) (accounts.User, error) {
	if s.UsernameStore == nil {
		// Fall back to standard validator if no UsernameStore
		return s.ValidateLocalCredentials(username, password, usernameType)
	}
	return localauth.NewCredentialsValidatorWithUsername(s.IdentityStore, s.ChannelStore, s.UserStore, s.UsernameStore)(username, password, usernameType)
}

// VerifyEmailByToken verifies an email using a verification token.
// Pass-through to localauth.NewVerifyEmailFunc.
func (s *AuthService) VerifyEmailByToken(token string) error {
	return localauth.NewVerifyEmailFunc(s.IdentityStore, s.TokenStore)(token)
}

// UpdatePassword updates the password for a user identified by email.
// Pass-through to localauth.NewUpdatePasswordFunc.
func (s *AuthService) UpdatePassword(email, newPassword string) error {
	return localauth.NewUpdatePasswordFunc(s.IdentityStore, s.ChannelStore)(email, newPassword)
}

// EnsureAuthUser handles user creation/lookup for both OAuth and local authentication.
// Pass-through to federatedauth.NewEnsureAuthUserFunc with channel linking support.
func (s *AuthService) EnsureAuthUser(authtype, provider string, token *oauth2.Token, userInfo map[string]any) (accounts.User, error) {
	return s.ensureUser(authtype, provider, token, userInfo)
}

// --- Store interface pass-throughs (for interface compliance) ---

// Implement accounts.UserStore interface.
func (s *AuthService) CreateUser(ctx context.Context, req *accounts.CreateUserRequest) (*accounts.CreateUserResponse, error) {
	return s.UserStore.CreateUser(ctx, req)
}

func (s *AuthService) GetUserById(ctx context.Context, req *accounts.GetUserByIDRequest) (*accounts.GetUserByIDResponse, error) {
	return s.UserStore.GetUserById(ctx, req)
}

func (s *AuthService) SaveUser(ctx context.Context, req *accounts.SaveUserRequest) (*accounts.SaveUserResponse, error) {
	return s.UserStore.SaveUser(ctx, req)
}

// Implement accounts.IdentityStore interface.
func (s *AuthService) GetIdentity(ctx context.Context, req *accounts.GetIdentityRequest) (*accounts.GetIdentityResponse, error) {
	return s.IdentityStore.GetIdentity(ctx, req)
}

func (s *AuthService) SaveIdentity(ctx context.Context, req *accounts.SaveIdentityRequest) (*accounts.SaveIdentityResponse, error) {
	return s.IdentityStore.SaveIdentity(ctx, req)
}

func (s *AuthService) SetUserForIdentity(ctx context.Context, req *accounts.SetUserForIdentityRequest) (*accounts.SetUserForIdentityResponse, error) {
	return s.IdentityStore.SetUserForIdentity(ctx, req)
}

func (s *AuthService) MarkIdentityVerified(ctx context.Context, req *accounts.MarkIdentityVerifiedRequest) (*accounts.MarkIdentityVerifiedResponse, error) {
	return s.IdentityStore.MarkIdentityVerified(ctx, req)
}

func (s *AuthService) GetUserIdentities(ctx context.Context, req *accounts.GetUserIdentitiesRequest) (*accounts.GetUserIdentitiesResponse, error) {
	return s.IdentityStore.GetUserIdentities(ctx, req)
}

// Implement accounts.ChannelStore interface.
func (s *AuthService) GetChannel(ctx context.Context, req *accounts.GetChannelRequest) (*accounts.GetChannelResponse, error) {
	return s.ChannelStore.GetChannel(ctx, req)
}

func (s *AuthService) SaveChannel(ctx context.Context, req *accounts.SaveChannelRequest) (*accounts.SaveChannelResponse, error) {
	return s.ChannelStore.SaveChannel(ctx, req)
}

func (s *AuthService) GetChannelsByIdentity(ctx context.Context, req *accounts.GetChannelsByIdentityRequest) (*accounts.GetChannelsByIdentityResponse, error) {
	return s.ChannelStore.GetChannelsByIdentity(ctx, req)
}
