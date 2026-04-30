// robot-simulator publishes synthetic telemetry to the AMQP broker.
//
// Every second it emits a JSON payload to routing key "telemetry.robot" on the
// "arrowhead" topic exchange.  Set AMQP_URL (default: local guest) and
// ROBOT_ID (default: robot-1) via environment variables.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	broker "arrowhead/message-broker"
)

type telemetryMsg struct {
	RobotID     string  `json:"robotId"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	Timestamp   string  `json:"timestamp"`
	Seq         int64   `json:"seq"`
}

func main() {
	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}
	robotID := os.Getenv("ROBOT_ID")
	if robotID == "" {
		robotID = "robot-1"
	}
	healthPort := os.Getenv("HEALTH_PORT")
	if healthPort == "" {
		healthPort = "9003"
	}

	// Serve a /health endpoint so the dashboard can poll this service.
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "system": "robot-simulator"})
		})
		log.Printf("[robot-simulator] health server on :%s", healthPort)
		log.Fatal(http.ListenAndServe(":"+healthPort, mux))
	}()

	// Retry connecting so the service survives a slow RabbitMQ startup.
	var (
		b   *broker.Broker
		err error
	)
	for attempts := 0; attempts < 15; attempts++ {
		b, err = broker.New(broker.Config{URL: amqpURL, Exchange: "arrowhead"})
		if err == nil {
			break
		}
		log.Printf("[robot-simulator] connect failed (attempt %d/15): %v — retrying in 2s", attempts+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("[robot-simulator] could not connect after 15 attempts: %v", err)
	}
	defer b.Close()

	log.Printf("[robot-simulator] connected — publishing as %s", robotID)

	var seq int64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Seed a simple pseudo-random walk for realistic-ish values.
	temp := 22.0
	hum := 55.0

	for range ticker.C {
		seq++
		temp += (float64(seq%3) - 1.0) * 0.1 // ±0.1 °C drift
		hum += (float64(seq%5) - 2.0) * 0.05  // ±0.1 % drift

		msg := telemetryMsg{
			RobotID:     robotID,
			Temperature: round2(temp),
			Humidity:    round2(hum),
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Seq:         seq,
		}
		payload, _ := json.Marshal(msg)
		if err := b.Publish("telemetry.robot", payload); err != nil {
			log.Printf("[robot-simulator] publish error: %v", err)
		} else {
			log.Printf("[robot-simulator] seq=%d temp=%.2f hum=%.2f", seq, temp, hum)
		}
	}
}

func round2(v float64) float64 {
	return float64(int(v*100)) / 100
}
