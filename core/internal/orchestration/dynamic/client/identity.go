package client

import "context"

// IdentityClient describes what DynamicOrchestration needs from the Authentication system.
type IdentityClient interface {
	// VerifyToken validates the given Bearer token and returns the verified systemName.
	// Returns ("", err) when the token is invalid or the auth system is unreachable.
	VerifyToken(ctx context.Context, token string) (string, error)
}
