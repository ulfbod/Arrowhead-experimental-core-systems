// fleet.go — manages N robot goroutines and applies per-robot 5G emulation.
package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	broker "arrowhead/message-broker"
)

// RobotConfig holds the identity and network profile for one simulated robot.
type RobotConfig struct {
	ID      string        `json:"id"`
	Network NetworkProfile `json:"network"`
}

// FleetConfig is the full configuration applied to the fleet.
type FleetConfig struct {
	PayloadType string        `json:"payloadType"` // "basic" | "imu"
	PayloadHz   float64       `json:"payloadHz"`   // 1–50
	Robots      []RobotConfig `json:"robots"`
}

// basicPayload is the minimal telemetry message (backward-compatible with
// the old robot-simulator format).
type basicPayload struct {
	RobotID     string  `json:"robotId"`
	Timestamp   string  `json:"timestamp"`
	Seq         int64   `json:"seq"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
}

// imuPayload is the extended message matching the physical Husqvarna ROS2 data.
type imuPayload struct {
	RobotID     string          `json:"robotId"`
	Timestamp   string          `json:"timestamp"`
	Seq         int64           `json:"seq"`
	IMU         imuData         `json:"imu"`
	Position    positionData    `json:"position"`
	Temperature float64         `json:"temperature"`
	Humidity    float64         `json:"humidity"`
}

type imuData struct {
	Roll  float64 `json:"roll"`
	Pitch float64 `json:"pitch"`
	Yaw   float64 `json:"yaw"`
}

type positionData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Fleet manages robot goroutines and exposes a snapshot of current stats.
type Fleet struct {
	mu       sync.RWMutex
	cfg      FleetConfig
	counters map[string]*robotCounter
	cancel   context.CancelFunc
	broker   *broker.Broker
}

// NewFleet creates and starts a fleet with the given initial config.
func NewFleet(b *broker.Broker, cfg FleetConfig) *Fleet {
	f := &Fleet{broker: b}
	f.apply(cfg)
	return f
}

// UpdateConfig stops existing robots and starts a new set with the new config.
func (f *Fleet) UpdateConfig(cfg FleetConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cancel != nil {
		f.cancel()
	}
	f.apply(cfg)
}

// apply must be called with f.mu held.
func (f *Fleet) apply(cfg FleetConfig) {
	if cfg.PayloadHz <= 0 {
		cfg.PayloadHz = 1
	}
	if cfg.PayloadType == "" {
		cfg.PayloadType = "imu"
	}
	f.cfg = cfg
	f.counters = make(map[string]*robotCounter, len(cfg.Robots))
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	for _, rc := range cfg.Robots {
		counter := &robotCounter{}
		f.counters[rc.ID] = counter
		go runRobot(ctx, f.broker, rc, cfg.PayloadHz, cfg.PayloadType, counter)
	}
}

// Config returns the current fleet configuration (copy).
func (f *Fleet) Config() FleetConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cfg
}

// Stats returns a snapshot of current send-side metrics.
func (f *Fleet) Stats() FleetStats {
	f.mu.RLock()
	snapshots := make(map[string]RobotStats, len(f.counters))
	for id, c := range f.counters {
		snapshots[id] = c.snapshot()
	}
	f.mu.RUnlock()

	agg := RobotStats{}
	for _, s := range snapshots {
		agg.MsgSent += s.MsgSent
		agg.MsgDropped += s.MsgDropped
		agg.BytesSent += s.BytesSent
		agg.RateHz += s.RateHz
		agg.KbpsSent += s.KbpsSent
	}
	return FleetStats{Robots: snapshots, Aggregate: agg}
}

// RobotCount returns the number of configured robots.
func (f *Fleet) RobotCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.cfg.Robots)
}

// runRobot is the goroutine for one simulated robot.
func runRobot(
	ctx context.Context,
	b *broker.Broker,
	rc RobotConfig,
	hz float64,
	payloadType string,
	counter *robotCounter,
) {
	profile := ResolveProfile(rc.Network)
	interval := time.Duration(float64(time.Second) / hz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Sensor state — pseudo-random walks.
	temp := 22.0
	hum := 55.0
	roll, pitch, yaw := 0.0, 0.0, 0.0
	x, y := 10.0, 5.0
	var seq int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			// Advance sensor state.
			temp += (randWalk() * 0.1)
			hum += (randWalk() * 0.1)
			roll += randWalk() * 0.02
			pitch += randWalk() * 0.02
			yaw += randWalk() * 0.05
			x += randWalk() * 0.1
			y += randWalk() * 0.1

			// Packet-loss drop.
			if ShouldDrop(profile) {
				counter.recordDropped()
				continue
			}

			ts := time.Now().UTC().Format(time.RFC3339Nano)

			var payload []byte
			var err error
			if payloadType == "basic" {
				payload, err = json.Marshal(basicPayload{
					RobotID:     rc.ID,
					Timestamp:   ts,
					Seq:         seq,
					Temperature: round2(temp),
					Humidity:    round2(hum),
				})
			} else {
				payload, err = json.Marshal(imuPayload{
					RobotID:   rc.ID,
					Timestamp: ts,
					Seq:       seq,
					IMU: imuData{
						Roll:  round3(roll),
						Pitch: round3(pitch),
						Yaw:   round3(yaw),
					},
					Position:    positionData{X: round2(x), Y: round2(y), Z: 0},
					Temperature: round2(temp),
					Humidity:    round2(hum),
				})
			}
			if err != nil {
				log.Printf("[robot-fleet/%s] marshal error: %v", rc.ID, err)
				continue
			}

			// Simulate 5G latency.
			if d := DelayDuration(profile); d > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(d):
				}
			}
			// Simulate bandwidth cap.
			if d := ByteDelay(profile, len(payload)); d > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(d):
				}
			}

			routingKey := "telemetry." + rc.ID
			if err := b.Publish(routingKey, payload); err != nil {
				log.Printf("[robot-fleet/%s] publish error: %v", rc.ID, err)
				continue
			}
			counter.recordSent(len(payload))
		}
	}
}

func randWalk() float64 {
	// Returns -1, 0, or +1 with equal probability.
	switch time.Now().UnixNano() % 3 {
	case 0:
		return -1
	case 1:
		return 0
	default:
		return 1
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
