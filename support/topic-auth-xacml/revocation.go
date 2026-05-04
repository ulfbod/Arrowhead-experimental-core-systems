// revocation.go — proactive revocation of AMQP connections for consumers
// whose grants have been removed from AuthzForce.
//
// Problem: RabbitMQ's HTTP auth backend is queried only on connection setup
// and queue.bind, not on each message delivery.  Once consumer-1 is connected
// and bound, revoking its CA grant has no immediate effect — messages keep
// flowing until the AMQP connection is terminated.
//
// Solution: a background loop that:
//  1. Lists all live AMQP connections from the RabbitMQ management API.
//  2. For each consumer connection (non-admin, non-publisher), asks AuthzForce
//     whether the grant still exists.
//  3. If the decision is Deny, deletes the AMQP connection via the management
//     API.  RabbitMQ terminates the TCP socket; consumer-direct's retry loop
//     reconnects — and /auth/user will deny it again if the grant is still gone.
//
// Propagation latency:
//   policy-sync syncs every SYNC_INTERVAL (default 10 s) → AuthzForce updated.
//   This loop runs every REVOCATION_INTERVAL (default 15 s).
//   Maximum end-to-end revocation delay: SYNC_INTERVAL + REVOCATION_INTERVAL ≈ 25 s.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type rmqConnection struct {
	Name string `json:"name"`
	User string `json:"user"`
}

type revocationChecker struct {
	srv      *authServer
	mgmtURL  string
	user     string
	pass     string
	interval time.Duration
	client   *http.Client
}

func newRevocationChecker(srv *authServer, mgmtURL, user, pass string, interval time.Duration) *revocationChecker {
	return &revocationChecker{
		srv:      srv,
		mgmtURL:  strings.TrimRight(mgmtURL, "/"),
		user:     user,
		pass:     pass,
		interval: interval,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (rc *revocationChecker) run() {
	log.Printf("[topic-auth-xacml] revocation loop every %s (mgmt: %s)", rc.interval, rc.mgmtURL)
	for {
		time.Sleep(rc.interval)
		if err := rc.checkOnce(); err != nil {
			log.Printf("[topic-auth-xacml] revocation check error: %v", err)
		}
	}
}

func (rc *revocationChecker) checkOnce() error {
	conns, err := rc.listConnections()
	if err != nil {
		return fmt.Errorf("listConnections: %w", err)
	}
	for _, conn := range conns {
		// Skip the admin and publisher — they always have access.
		if conn.User == rc.srv.cfg.rmqAdminUser || conn.User == rc.srv.cfg.publisherUser {
			continue
		}
		permit, err := rc.srv.decide(context.Background(), conn.User, "telemetry", "subscribe")
		if err != nil {
			log.Printf("[topic-auth-xacml] revocation: decide error user=%q: %v", conn.User, err)
			continue
		}
		if !permit {
			log.Printf("[topic-auth-xacml] revocation: CLOSING connection user=%q name=%q (grant revoked)",
				conn.User, conn.Name)
			if err := rc.closeConnection(conn.Name); err != nil {
				log.Printf("[topic-auth-xacml] revocation: close error: %v", err)
			}
		}
	}
	return nil
}

func (rc *revocationChecker) listConnections() ([]rmqConnection, error) {
	req, err := http.NewRequest(http.MethodGet, rc.mgmtURL+"/api/connections", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(rc.user, rc.pass)
	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET /api/connections %d: %s", resp.StatusCode, b)
	}
	var conns []rmqConnection
	return conns, json.NewDecoder(resp.Body).Decode(&conns)
}

// closeConnection terminates a single AMQP connection by its name.
// The name is URL-path-encoded because it contains spaces and punctuation
// (e.g. "172.19.0.5:42314 -> 172.19.0.3:5672").
func (rc *revocationChecker) closeConnection(name string) error {
	encoded := url.PathEscape(name)
	req, err := http.NewRequest(http.MethodDelete, rc.mgmtURL+"/api/connections/"+encoded, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(rc.user, rc.pass)
	resp, err := rc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE connection %d: %s", resp.StatusCode, b)
	}
	return nil
}
