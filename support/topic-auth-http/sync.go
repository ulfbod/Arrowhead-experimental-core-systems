package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type config struct {
	consumerAuthURL  string
	rmqBase          string
	rmqAdminUser     string
	rmqAdminPass     string
	rmqVhost         string
	rmqExchange      string
	consumerPassword string
	publisherUser    string
	publisherPass    string
}

type syncer struct {
	cfg       config
	rmq       *rmqClient
	authToken string
}

// setToken stores the Bearer token to be used on ConsumerAuth API calls.
func (s *syncer) setToken(token string) {
	s.authToken = token
}

func newSyncer(cfg config, rmq *rmqClient) *syncer {
	return &syncer{cfg: cfg, rmq: rmq}
}

// sync performs one full reconciliation cycle: fetches CA rules, ensures the
// publisher user exists, creates/updates consumer users, and deletes stale ones.
// This is the safety-net path: the HTTP auth backend handles per-operation
// authorization live; sync handles user lifecycle so that authn (internal
// backend) continues to work and stale users are eventually cleaned up.
func (s *syncer) sync() error {
	rules, err := s.fetchRules()
	if err != nil {
		return fmt.Errorf("fetchRules: %w", err)
	}
	log.Printf("[sync] fetched %d rules from ConsumerAuth", len(rules))

	if err := s.ensurePublisher(); err != nil {
		return fmt.Errorf("ensurePublisher: %w", err)
	}

	desired := BuildDesiredUsers(rules, s.cfg.rmqExchange)
	for username, tp := range desired {
		if err := s.rmq.ensureUser(username, s.cfg.consumerPassword); err != nil {
			return fmt.Errorf("ensureUser %q: %w", username, err)
		}
		perm := rmqPermission{Configure: ".*", Write: ".*", Read: ".*"}
		if err := s.rmq.setPermissions(username, perm); err != nil {
			return fmt.Errorf("setPermissions %q: %w", username, err)
		}
		topicPerm := rmqTopicPermission{Exchange: tp.Exchange, Write: tp.Write, Read: tp.Read}
		if err := s.rmq.setTopicPermission(username, topicPerm); err != nil {
			return fmt.Errorf("setTopicPermission %q: %w", username, err)
		}
		log.Printf("[sync] ensured user %q with topic read=%q", username, tp.Read)
	}

	managed, err := s.rmq.listManagedUsers()
	if err != nil {
		return fmt.Errorf("listManagedUsers: %w", err)
	}
	for _, username := range managed {
		if username == s.cfg.publisherUser {
			continue
		}
		if _, ok := desired[username]; !ok {
			if err := s.rmq.deleteUser(username); err != nil {
				return fmt.Errorf("deleteUser %q: %w", username, err)
			}
			log.Printf("[sync] deleted stale user %q", username)
		}
	}
	return nil
}

// fetchRules calls GET {consumerAuthURL}/authorization/lookup and returns the rules.
// If an authToken is set, it is attached as a Bearer header.
func (s *syncer) fetchRules() ([]AuthRule, error) {
	req, err := http.NewRequest(http.MethodGet, s.cfg.consumerAuthURL+"/authorization/lookup", nil)
	if err != nil {
		return nil, err
	}
	if s.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.authToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ConsumerAuth lookup returned %d", resp.StatusCode)
	}
	var lr LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, err
	}
	return lr.Rules, nil
}

// ensurePublisher creates or updates the publisher user with topic write permission.
func (s *syncer) ensurePublisher() error {
	username := s.cfg.publisherUser
	if err := s.rmq.ensureUser(username, s.cfg.publisherPass); err != nil {
		return err
	}
	perm := rmqPermission{Configure: ".*", Write: ".*", Read: ".*"}
	if err := s.rmq.setPermissions(username, perm); err != nil {
		return err
	}
	tp := PublisherPermission(s.cfg.rmqExchange, []string{"telemetry"})
	topicPerm := rmqTopicPermission{Exchange: tp.Exchange, Write: tp.Write, Read: tp.Read}
	if err := s.rmq.setTopicPermission(username, topicPerm); err != nil {
		return err
	}
	log.Printf("[sync] ensured publisher %q", username)
	return nil
}

// enforceRevocations fetches current CA grants and force-closes any active
// AMQP connection belonging to a consumer whose grant has been revoked.
// The publisher and admin users are always skipped.
func (s *syncer) enforceRevocations() error {
	if s.rmq == nil {
		return nil
	}
	rules, err := s.fetchRules()
	if err != nil {
		return fmt.Errorf("fetchRules: %w", err)
	}
	conns, err := s.rmq.listConnections()
	if err != nil {
		return fmt.Errorf("listConnections: %w", err)
	}
	for _, conn := range conns {
		user := conn.User
		if user == "" || user == s.cfg.rmqAdminUser || user == s.cfg.publisherUser {
			continue
		}
		if !hasAnyGrant(rules, user) {
			if err := s.rmq.deleteConnection(conn.Name); err != nil {
				log.Printf("[revoke] close connection for %q: %v", user, err)
			} else {
				log.Printf("[revoke] closed AMQP connection for revoked consumer %q", user)
			}
		}
	}
	return nil
}
