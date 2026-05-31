// Package model defines types for the Translation Manager core system.
package model

import "encoding/json"

// Bridge describes a field-remapping configuration between two data formats.
type Bridge struct {
	ID            string            `json:"id"`
	SourceFormat  string            `json:"sourceFormat"`
	TargetFormat  string            `json:"targetFormat"`
	FieldMappings map[string]string `json:"fieldMappings"`
	Active        bool              `json:"active"`
	CreatedAt     string            `json:"createdAt"`
}

// TranslateRequest is the body for POST /translationmanager/translation/translate.
type TranslateRequest struct {
	BridgeID string          `json:"bridgeId"`
	Payload  json.RawMessage `json:"payload"`
}

// TranslateResponse is returned by POST /translationmanager/translation/translate.
type TranslateResponse struct {
	BridgeID          string          `json:"bridgeId"`
	OriginalPayload   json.RawMessage `json:"originalPayload"`
	TranslatedPayload json.RawMessage `json:"translatedPayload"`
}
