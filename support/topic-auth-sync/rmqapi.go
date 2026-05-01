package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const managedTag = "arrowhead-managed"

type rmqClient struct {
	base  string
	user  string
	pass  string
	vhost string
}

type rmqUserBody struct {
	Password string `json:"password"`
	Tags     string `json:"tags"`
}

// rmqUserResp models the GET /api/users response.
// RabbitMQ 3.12+ returns tags as a JSON array; older versions returned a
// comma-separated string. We use []string to handle the modern format.
type rmqUserResp struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type rmqPermission struct {
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

type rmqTopicPermission struct {
	Exchange string `json:"exchange"`
	Write    string `json:"write"`
	Read     string `json:"read"`
}

func newRMQClient(base, user, pass, vhost string) *rmqClient {
	return &rmqClient{base: base, user: user, pass: pass, vhost: vhost}
}

// vhostPath returns the URL-escaped vhost for use in API paths.
func (c *rmqClient) vhostPath() string {
	return url.PathEscape(c.vhost)
}

// listManagedUsers returns the names of all users tagged arrowhead-managed.
func (c *rmqClient) listManagedUsers() ([]string, error) {
	var users []rmqUserResp
	if err := c.doGET("/api/users", &users); err != nil {
		return nil, err
	}
	var managed []string
	for _, u := range users {
		if containsTag(u.Tags, managedTag) {
			managed = append(managed, u.Name)
		}
	}
	return managed, nil
}

// ensureUser creates or updates a managed user.
func (c *rmqClient) ensureUser(username, password string) error {
	body := rmqUserBody{
		Password: password,
		Tags:     managedTag,
	}
	return c.doPUT("/api/users/"+url.PathEscape(username), body)
}

// deleteUser removes a user from RabbitMQ.
func (c *rmqClient) deleteUser(username string) error {
	return c.doDELETE("/api/users/" + url.PathEscape(username))
}

// setPermissions sets the regular (non-topic) permissions for a user on the configured vhost.
func (c *rmqClient) setPermissions(username string, p rmqPermission) error {
	path := fmt.Sprintf("/api/permissions/%s/%s", c.vhostPath(), url.PathEscape(username))
	return c.doPUT(path, p)
}

// setTopicPermission sets the topic permission for a user on the configured vhost.
func (c *rmqClient) setTopicPermission(username string, tp rmqTopicPermission) error {
	path := fmt.Sprintf("/api/topic-permissions/%s/%s", c.vhostPath(), url.PathEscape(username))
	return c.doPUT(path, tp)
}

// deleteTopicPermissions removes all topic permissions for a user on the configured vhost.
func (c *rmqClient) deleteTopicPermissions(username string) error {
	path := fmt.Sprintf("/api/topic-permissions/%s/%s", c.vhostPath(), url.PathEscape(username))
	return c.doDELETE(path)
}

// doGET performs an authenticated GET and JSON-decodes the response body into out.
func (c *rmqClient) doGET(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: unexpected status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// doPUT performs an authenticated PUT with a JSON body.
func (c *rmqClient) doPUT(path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PUT %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
}

// doDELETE performs an authenticated DELETE request.
func (c *rmqClient) doDELETE(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.base+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DELETE %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
}

// containsTag reports whether target appears in the tags slice.
func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}
