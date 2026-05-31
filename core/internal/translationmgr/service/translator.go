// Package service implements Translation Manager business logic.
package service

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"arrowhead/core/internal/translationmgr/model"
)

var (
	ErrBridgeNotFound = errors.New("bridge not found")
	ErrDuplicateBridge = errors.New("bridge already exists")
)

// TranslationService manages bridges and performs field-remapping translations.
type TranslationService struct {
	mu      sync.RWMutex
	bridges map[string]*model.Bridge
}

// NewTranslationService creates a new empty TranslationService.
func NewTranslationService() *TranslationService {
	return &TranslationService{bridges: make(map[string]*model.Bridge)}
}

// CreateBridge stores a new translation bridge.
func (s *TranslationService) CreateBridge(b model.Bridge) (*model.Bridge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	if _, exists := s.bridges[b.ID]; exists {
		return nil, ErrDuplicateBridge
	}
	b.Active = true
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.bridges[b.ID] = &b
	return &b, nil
}

// GetBridge returns the bridge with the given ID.
func (s *TranslationService) GetBridge(id string) (*model.Bridge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bridges[id]
	if !ok {
		return nil, ErrBridgeNotFound
	}
	return b, nil
}

// ListBridges returns all stored bridges.
func (s *TranslationService) ListBridges() []*model.Bridge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*model.Bridge, 0, len(s.bridges))
	for _, b := range s.bridges {
		out = append(out, b)
	}
	return out
}

// DeleteBridge removes a bridge by ID.
func (s *TranslationService) DeleteBridge(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bridges[id]; !ok {
		return ErrBridgeNotFound
	}
	delete(s.bridges, id)
	return nil
}

// Translate applies the field mappings of bridge bridgeID to payload.
// Returns ErrBridgeNotFound if the bridge does not exist.
func (s *TranslationService) Translate(bridgeID string, payload json.RawMessage) (model.TranslateResponse, error) {
	bridge, err := s.GetBridge(bridgeID)
	if err != nil {
		return model.TranslateResponse{}, err
	}
	// Unmarshal the input payload as a map.
	var input map[string]json.RawMessage
	if err := json.Unmarshal(payload, &input); err != nil {
		return model.TranslateResponse{}, err
	}
	// Apply field mappings: rename keys according to bridge.FieldMappings.
	output := make(map[string]json.RawMessage, len(input))
	for srcKey, v := range input {
		if dstKey, ok := bridge.FieldMappings[srcKey]; ok {
			output[dstKey] = v
		} else {
			output[srcKey] = v
		}
	}
	translated, err := json.Marshal(output)
	if err != nil {
		return model.TranslateResponse{}, err
	}
	return model.TranslateResponse{
		BridgeID:          bridgeID,
		OriginalPayload:   payload,
		TranslatedPayload: json.RawMessage(translated),
	}, nil
}
