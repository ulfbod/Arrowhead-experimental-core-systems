package main

import (
	"encoding/json"
	"fmt"
	"log"
	"mineio/internal/common"
	"net/http"
	"os"
	"sync"
	"time"
)

// CommandRecord stores a single issued command for history.
type CommandRecord struct {
	ID             string    `json:"id"`
	TargetMachineID string   `json:"targetMachineId"`
	Command        string    `json:"command"`
	AuthLevel      string    `json:"authLevel"`
	IssuedAt       time.Time `json:"issuedAt"`
	Operator       bool      `json:"operatorPresent"`
}

type TeleRemoteService struct {
	mu                  sync.RWMutex
	state               common.TeleRemoteState
	commandHistory      []CommandRecord
	interventionActive  bool
	interventionTarget  string
	ah                  *common.ArrowheadClient
}

func main() {
	id := envOrDefault("IDT_ID", "idt4")
	name := envOrDefault("IDT_NAME", "Tele-Remote Station")
	port := envOrDefault("PORT", "8401")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &TeleRemoteService{
		ah: common.NewArrowheadClient(ahURL, id),
		state: common.TeleRemoteState{
			ID:                 id,
			Name:               name,
			Online:             true,
			OperatorPresent:    false,
			OverrideActive:     false,
			TargetMachineID:    "",
			AuthorizationLevel: "standard",
			LastCommand:        "",
			LastCommandTime:    time.Now(),
			LastUpdated:        time.Now(),
		},
		commandHistory:     []CommandRecord{},
		interventionActive: false,
		interventionTarget: "",
	}

	portInt := 8401
	fmt.Sscanf(port, "%d", &portInt)

	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "iDT",
		Capabilities: []string{"tele-operation", "operator-override", "remote-control", "intervention"},
		Metadata:     map[string]string{"machine": "tele-remote-station"},
	}, 10)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, 200, map[string]interface{}{"status": "ok", "id": id, "timestamp": time.Now()})
	})
	mux.HandleFunc("/state", svc.handleState)
	mux.HandleFunc("/command/override", svc.handleCommandOverride)
	mux.HandleFunc("/operator", svc.handleOperator)
	mux.HandleFunc("/override", svc.handleOverride)
	mux.HandleFunc("/commands", svc.handleCommands)
	mux.HandleFunc("/intervention/start", svc.handleInterventionStart)
	mux.HandleFunc("/intervention/stop", svc.handleInterventionStop)
	mux.HandleFunc("/online", svc.handleOnline)

	handler := common.CORSMiddleware(mux)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func (s *TeleRemoteService) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	common.WriteJSON(w, 200, state)
}

func (s *TeleRemoteService) handleCommandOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		TargetMachineID string `json:"targetMachineId"`
		Command         string `json:"command"`
		AuthLevel       string `json:"authLevel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, 400, "invalid JSON")
		return
	}

	s.mu.Lock()
	if !s.state.OperatorPresent {
		s.mu.Unlock()
		common.WriteError(w, 403, "operator not present: override requires operator at station")
		return
	}
	if body.Command == "" {
		s.mu.Unlock()
		common.WriteError(w, 400, "command is required")
		return
	}
	if body.AuthLevel == "" {
		body.AuthLevel = "standard"
	}
	if body.TargetMachineID == "" {
		body.TargetMachineID = s.state.TargetMachineID
	}

	// Require elevated or emergency auth for override commands
	if body.AuthLevel == "standard" && s.state.AuthorizationLevel == "standard" {
		s.mu.Unlock()
		common.WriteError(w, 403, "elevated or emergency authorization required for override commands")
		return
	}

	record := CommandRecord{
		ID:              fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		TargetMachineID: body.TargetMachineID,
		Command:         body.Command,
		AuthLevel:       body.AuthLevel,
		IssuedAt:        time.Now(),
		Operator:        s.state.OperatorPresent,
	}

	s.state.LastCommand = fmt.Sprintf("%s -> %s", body.Command, body.TargetMachineID)
	s.state.LastCommandTime = time.Now()
	s.state.TargetMachineID = body.TargetMachineID
	s.state.OverrideActive = true
	s.state.AuthorizationLevel = body.AuthLevel
	s.state.LastUpdated = time.Now()

	s.commandHistory = append(s.commandHistory, record)
	if len(s.commandHistory) > 20 {
		s.commandHistory = s.commandHistory[len(s.commandHistory)-20:]
	}
	id := s.state.ID
	s.mu.Unlock()

	log.Printf("[%s] Override command issued: %s -> %s (auth: %s)", id, body.Command, body.TargetMachineID, body.AuthLevel)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":          "override issued",
		"commandId":       record.ID,
		"targetMachineId": body.TargetMachineID,
		"command":         body.Command,
		"authLevel":       body.AuthLevel,
		"issuedAt":        record.IssuedAt,
	})
}

func (s *TeleRemoteService) handleOperator(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Present bool `json:"present"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.state.OperatorPresent = body.Present
	if !body.Present {
		// Auto-deactivate override when operator leaves
		s.state.OverrideActive = false
		s.state.AuthorizationLevel = "standard"
	}
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Operator present: %v", id, body.Present)
	common.WriteJSON(w, 200, map[string]bool{"operatorPresent": body.Present})
}

func (s *TeleRemoteService) handleOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Active          bool   `json:"active"`
		TargetMachineID string `json:"targetMachineId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	if body.Active && !s.state.OperatorPresent {
		s.mu.Unlock()
		common.WriteError(w, 403, "operator not present: cannot activate override")
		return
	}
	s.state.OverrideActive = body.Active
	if body.TargetMachineID != "" {
		s.state.TargetMachineID = body.TargetMachineID
	}
	if !body.Active {
		s.state.TargetMachineID = ""
	}
	s.state.LastUpdated = time.Now()
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Override active: %v, target: %s", id, body.Active, body.TargetMachineID)
	common.WriteJSON(w, 200, map[string]interface{}{
		"overrideActive":  body.Active,
		"targetMachineId": body.TargetMachineID,
	})
}

func (s *TeleRemoteService) handleCommands(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	history := make([]CommandRecord, len(s.commandHistory))
	copy(history, s.commandHistory)
	s.mu.RUnlock()

	// Return most recent first
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	common.WriteJSON(w, 200, map[string]interface{}{
		"commands": history,
		"count":    len(history),
	})
}

func (s *TeleRemoteService) handleInterventionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		TargetMachineID string `json:"targetMachineId"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	s.mu.Lock()
	if !s.state.OperatorPresent {
		s.mu.Unlock()
		common.WriteError(w, 403, "operator not present: intervention requires operator at station")
		return
	}
	s.interventionActive = true
	s.interventionTarget = body.TargetMachineID
	s.state.TargetMachineID = body.TargetMachineID
	s.state.OverrideActive = true
	s.state.AuthorizationLevel = "elevated"
	s.state.LastUpdated = time.Now()
	id := s.state.ID
	s.mu.Unlock()

	log.Printf("[%s] Intervention session started -> %s", id, body.TargetMachineID)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":          "intervention started",
		"targetMachineId": body.TargetMachineID,
		"authLevel":       "elevated",
		"startedAt":       time.Now(),
	})
}

func (s *TeleRemoteService) handleInterventionStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	target := s.interventionTarget
	s.interventionActive = false
	s.interventionTarget = ""
	s.state.OverrideActive = false
	s.state.TargetMachineID = ""
	s.state.AuthorizationLevel = "standard"
	s.state.LastUpdated = time.Now()
	id := s.state.ID
	s.mu.Unlock()

	log.Printf("[%s] Intervention session ended (was targeting %s)", id, target)
	common.WriteJSON(w, 200, map[string]interface{}{
		"status":    "intervention stopped",
		"stoppedAt": time.Now(),
	})
}

func (s *TeleRemoteService) handleOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Online bool `json:"online"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.state.Online = body.Online
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Online set to %v", id, body.Online)
	common.WriteJSON(w, 200, map[string]bool{"online": body.Online})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
