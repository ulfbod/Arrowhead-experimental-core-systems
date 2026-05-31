// Package client — TranslationClient interface for the Translation Manager support system.
package client

import (
	"context"
)

// TranslationClient is the interface for the Translation Manager support system (G36).
// When ALLOW_TRANSLATION is set in an orchestration request, providers that fail
// interface matching are re-evaluated via CanTranslate. If translation is possible,
// the provider is included in the result.
type TranslationClient interface {
	CanTranslate(ctx context.Context, fromIface, toIface string) (bool, error)
}

// NopTranslationClient is a no-op TranslationClient that always returns false.
// It is the default when TRANSLATION_MGR_URL is not configured.
type NopTranslationClient struct{}

// CanTranslate always returns false — translation is not available.
func (NopTranslationClient) CanTranslate(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
