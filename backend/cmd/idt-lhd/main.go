package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"mineio/internal/common"
	"net/http"
	"os"
	"sync"
	"time"
)

type LHDService struct {
	mu              sync.RWMutex
	state           common.LHDState
	clearanceActive bool
	ah              *common.ArrowheadClient
}

func main() {
	id := envOrDefault("IDT_ID", "idt3a")
	name := envOrDefault("IDT_NAME", "LHD Vehicle A")
	port := envOrDefault("PORT", "8301")
	ahURL := envOrDefault("ARROWHEAD_URL", "http://localhost:8000")
	host := envOrDefault("HOST", "localhost")

	log.Printf("[%s] Starting %s on :%s", id, name, port)

	svc := &LHDService{
		ah: common.NewArrowheadClient(ahURL, id),
		state: common.LHDState{
			ID:               id,
			Name:             name,
			Online:           true,
			Connected:        true,
			Position:         common.Position{X: rand.Float64() * 100, Y: rand.Float64() * 100, Z: 0},
			PayloadTons:      0,
			MaxPayloadTons:   15,
			Available:        true,
			TrammingStatus:   "idle",
			DebrisClearedPct: 0,
			FuelPct:          100,
			LastUpdated:      time.Now(),
		},
		clearanceActive: false,
	}

	portInt := 8301
	fmt.Sscanf(port, "%d", &portInt)

	common.RegisterWithRetry(svc.ah, common.RegisterRequest{
		ID:           id,
		Name:         name,
		Address:      host,
		Port:         portInt,
		ServiceType:  "iDT",
		Capabilities: []string{"debris-clearance", "tramming", "payload-handling", "material-handling"},
		Metadata:     map[string]string{"machine": "lhd-vehicle"},
	}, 10)

	go svc.simulate()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, 200, map[string]interface{}{"status": "ok", "id": id, "timestamp": time.Now()})
	})
	mux.HandleFunc("/state", svc.handleState)
	mux.HandleFunc("/tram", svc.handleTram)
	mux.HandleFunc("/tram/stop", svc.handleTramStop)
	mux.HandleFunc("/clearance/start", svc.handleClearanceStart)
	mux.HandleFunc("/clearance/stop", svc.handleClearanceStop)
	mux.HandleFunc("/clearance/status", svc.handleClearanceStatus)
	mux.HandleFunc("/availability", svc.handleAvailability)
	mux.HandleFunc("/connectivity", svc.handleConnectivity)
	mux.HandleFunc("/online", svc.handleOnline)
	mux.HandleFunc("/simulate/reset", svc.handleSimulateReset)

	handler := common.CORSMiddleware(mux)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func (s *LHDService) simulate() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		if s.state.Online && s.state.Connected {
			// Fuel drain when active
			if s.state.TrammingStatus != "idle" && s.state.FuelPct > 0 {
				s.state.FuelPct = math.Max(0, s.state.FuelPct-0.02)
			}

			// Debris clearance progression
			if s.clearanceActive && s.state.TrammingStatus == "tramming" && s.state.DebrisClearedPct < 100 {
				s.state.DebrisClearedPct = math.Min(100, s.state.DebrisClearedPct+0.05)
			}

			// Payload simulation: gradually fill during loading, empty during unloading
			switch s.state.TrammingStatus {
			case "loading":
				if s.state.PayloadTons < s.state.MaxPayloadTons {
					s.state.PayloadTons = math.Min(s.state.MaxPayloadTons, s.state.PayloadTons+0.1)
				} else {
					// Auto-transition to tramming once full
					s.state.TrammingStatus = "tramming"
				}
			case "unloading":
				if s.state.PayloadTons > 0 {
					s.state.PayloadTons = math.Max(0, s.state.PayloadTons-0.2)
				} else {
					// Auto-transition to idle once empty
					s.state.TrammingStatus = "idle"
				}
			case "tramming":
				// Position drift
				s.state.Position.X += (rand.Float64() - 0.5) * 3
				s.state.Position.Y += (rand.Float64() - 0.5) * 3
				s.state.Position.X = math.Max(0, math.Min(200, s.state.Position.X))
				s.state.Position.Y = math.Max(0, math.Min(200, s.state.Position.Y))
			}

			s.state.LastUpdated = time.Now()
		}
		s.mu.Unlock()
	}
}

func (s *LHDService) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	common.WriteJSON(w, 200, state)
}

func (s *LHDService) handleTram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	var body struct {
		Destination string `json:"destination"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Destination == "" {
		body.Destination = "zone-a"
	}
	s.mu.Lock()
	s.state.TrammingStatus = "tramming"
	s.state.Available = false
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Tramming to %s", id, body.Destination)
	common.WriteJSON(w, 200, map[string]interface{}{"status": "tramming", "destination": body.Destination})
}

func (s *LHDService) handleTramStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	s.state.TrammingStatus = "idle"
	s.state.Available = true
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Tramming stopped", id)
	common.WriteJSON(w, 200, map[string]string{"status": "idle"})
}

func (s *LHDService) handleClearanceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	s.clearanceActive = true
	s.state.TrammingStatus = "tramming"
	s.state.Available = false
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Debris clearance started", id)
	common.WriteJSON(w, 200, map[string]string{"status": "clearance started"})
}

func (s *LHDService) handleClearanceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	s.clearanceActive = false
	s.state.TrammingStatus = "idle"
	s.state.Available = true
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Debris clearance stopped", id)
	common.WriteJSON(w, 200, map[string]string{"status": "clearance stopped"})
}

func (s *LHDService) handleClearanceStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cleared := s.state.DebrisClearedPct
	active := s.clearanceActive
	s.mu.RUnlock()

	remaining := 100 - cleared
	// Estimate: 0.05%/sec -> seconds remaining, convert to minutes
	var etaMinutes int
	if active && remaining > 0 {
		etaMinutes = int(math.Ceil(remaining / 0.05 / 60))
	}

	common.WriteJSON(w, 200, map[string]interface{}{
		"debrisClearedPct":      cleared,
		"estimatedEtaMinutes":   etaMinutes,
		"routeClear":            cleared >= 100,
		"clearanceActive":       active,
	})
}

func (s *LHDService) handleAvailability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Available bool `json:"available"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.state.Available = body.Available
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]bool{"available": body.Available})
}

func (s *LHDService) handleConnectivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		common.WriteError(w, 405, "PUT required")
		return
	}
	var body struct {
		Connected bool `json:"connected"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.state.Connected = body.Connected
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]bool{"connected": body.Connected})
}

func (s *LHDService) handleOnline(w http.ResponseWriter, r *http.Request) {
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
	s.mu.Unlock()
	common.WriteJSON(w, 200, map[string]bool{"online": body.Online})
}

func (s *LHDService) handleSimulateReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, 405, "POST required")
		return
	}
	s.mu.Lock()
	s.state.DebrisClearedPct = 0
	s.state.PayloadTons = 0
	s.state.TrammingStatus = "idle"
	s.clearanceActive = false
	s.state.Available = true
	id := s.state.ID
	s.mu.Unlock()
	log.Printf("[%s] Simulation reset: debris cleared to 0%%", id)
	common.WriteJSON(w, 200, map[string]string{"status": "reset", "message": "debris scenario reset to 0%"})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
