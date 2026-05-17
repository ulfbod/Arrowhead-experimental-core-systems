// revocation.go — proactive revocation of AMQP connections for consumers
// whose grants have been removed from AuthzForce.
//
// This is a local copy of support/topic-auth-xacml/revocation.go adapted to
// use the enriched XACML decide function with PIP cert-level attributes.
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
		if conn.User == rc.srv.cfg.rmqAdminUser || rc.srv.isPublisher(conn.User) {
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
