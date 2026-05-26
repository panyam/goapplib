package services_test

import (
	"github.com/panyam/goapplib/services"
	"github.com/panyam/oneauth/accounts"
	"github.com/panyam/oneauth/federatedauth"
)

// Compile-time assertion that *AuthService satisfies the oneauth interfaces
// downstream consumers (notation, lilbattle) rely on. Without these, a stale
// import path or signature drift would only surface in those repos.
var (
	_ accounts.UserStore           = (*services.AuthService)(nil)
	_ accounts.IdentityStore       = (*services.AuthService)(nil)
	_ accounts.ChannelStore        = (*services.AuthService)(nil)
	_ federatedauth.AuthUserStore  = (*services.AuthService)(nil)
)
