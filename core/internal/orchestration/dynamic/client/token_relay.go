package client

import (
	"context"

	orchmodel "arrowhead/core/internal/orchestration/model"
)

// TokenRelayClient describes what DynamicOrchestration needs to relay ConsumerAuth tokens
// into OrchestrationResult entries (G54, design decision D11).
//
// GenerateToken requests a time-limited authorization token from ConsumerAuthorization for
// the given consumer/provider/serviceDefinition tuple. The returned descriptor is embedded
// in the result's AuthorizationToken map under the SecurityPolicy key (outer) and the
// service definition name (inner), per the AH5 AuthorizationTokenMap structure.
type TokenRelayClient interface {
	GenerateToken(
		ctx context.Context,
		consumer, provider, serviceDefinition string,
		tokenVariant string,
	) (*orchmodel.AuthorizationTokenDescriptor, error)
}
