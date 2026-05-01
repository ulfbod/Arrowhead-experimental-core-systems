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
	cfg config
	rmq *rmqClient
}

func newSyncer(cfg config) *syncer {
	rmq := newRMQClient(cfg.rmqBase, cfg.rmqAdminUser, cfg.rmqAdminPass, cfg.rmqVhost)
	return &syncer{cfg: cfg, rmq: rmq}
}

// sync performs one full reconciliation cycle.
func (s *syncer) sync() error {
	// 1. Fetch current rules from ConsumerAuth.
	rules, err := s.fetchRules()
	if err != nil {
		return fmt.Errorf("fetchRules: %w", err)
	}
	log.Printf("[sync] fetched %d rules from ConsumerAuth", len(rules))

	// 2. Ensure publisher user exists with correct topic permissions.
	if err := s.ensurePublisher(); err != nil {
		return fmt.Errorf("ensurePublisher: %w", err)
	}

	// 3. Build desired consumer users and apply permissions.
	desired := BuildDesiredUsers(rules, s.cfg.rmqExchange)
	for username, tp := range desired {
		if err := s.rmq.ensureUser(username, s.cfg.consumerPassword); err != nil {
			return fmt.Errorf("ensureUser %q: %w", username, err)
		}
		log.Printf("[sync] ensured user %q", username)

		// Regular permissions: consumers need configure access to declare the exchange
		// and their own queue, and read access to consume from it.
		perm := rmqPermission{
			Configure: ".*",
			Write:     ".*",
			Read:      ".*",
		}
		if err := s.rmq.setPermissions(username, perm); err != nil {
			return fmt.Errorf("setPermissions %q: %w", username, err)
		}

		// Topic permissions: restrict which routing keys the consumer may bind to.
		topicPerm := rmqTopicPermission{
			Exchange: tp.Exchange,
			Write:    tp.Write,
			Read:     tp.Read,
		}
		if err := s.rmq.setTopicPermission(username, topicPerm); err != nil {
			return fmt.Errorf("setTopicPermission %q: %w", username, err)
		}
		log.Printf("[sync] set topic permission for %q: read=%q", username, tp.Read)
	}

	// 4. Remove stale managed users that are no longer in the desired set.
	// The publisher user is managed separately and must never be treated as stale.
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
func (s *syncer) fetchRules() ([]AuthRule, error) {
	resp, err := http.Get(s.cfg.consumerAuthURL + "/authorization/lookup")
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

// ensurePublisher creates or updates the publisher user with its topic write permission.
func (s *syncer) ensurePublisher() error {
	username := s.cfg.publisherUser

	if err := s.rmq.ensureUser(username, s.cfg.publisherPass); err != nil {
		return err
	}
	log.Printf("[sync] ensured publisher user %q", username)

	// Publisher gets full regular permissions.
	perm := rmqPermission{
		Configure: ".*",
		Write:     ".*",
		Read:      ".*",
	}
	if err := s.rmq.setPermissions(username, perm); err != nil {
		return err
	}

	// Publisher topic permission: may write telemetry routing keys.
	tp := PublisherPermission(s.cfg.rmqExchange, []string{"telemetry"})
	topicPerm := rmqTopicPermission{
		Exchange: tp.Exchange,
		Write:    tp.Write,
		Read:     tp.Read,
	}
	if err := s.rmq.setTopicPermission(username, topicPerm); err != nil {
		return err
	}
	log.Printf("[sync] set topic permission for publisher %q: write=%q", username, tp.Write)

	return nil
}
