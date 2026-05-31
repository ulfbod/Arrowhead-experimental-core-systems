package client

import "context"

// ConsumerAuthClient describes what DynamicOrchestration needs from ConsumerAuthorization.
type ConsumerAuthClient interface {
	IsAuthorized(ctx context.Context, consumerSystemName, providerSystemName, serviceDefinition string) (bool, error)
}
