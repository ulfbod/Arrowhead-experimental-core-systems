// Package client provides interface-backed HTTP clients for DynamicOrchestration.
package client

import (
	"context"

	orchmodel "arrowhead/core/internal/orchestration/model"
)

// ServiceRegistryClient describes what DynamicOrchestration needs from the ServiceRegistry.
type ServiceRegistryClient interface {
	LookupServices(ctx context.Context, req orchmodel.OrchestrationRequest) ([]orchmodel.OrchestrationResult, error)
}
