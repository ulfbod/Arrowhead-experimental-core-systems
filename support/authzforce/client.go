// Package authzforce provides a Go client for the AuthzForce CE REST API.
//
// AuthzForce CE implements the XACML REST Profile (OASIS). All communication
// uses XML. The key operations are:
//   - EnsureDomain — create or look up a policy domain by external ID
//   - SetPolicy    — upload/replace a XACML 3.0 PolicySet in the domain PAP
//   - Decide       — evaluate a XACML 3.0 access-control request via the domain PDP
package authzforce

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// XACML 3.0 namespace
	xacmlNS = "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"

	// AuthzForce REST model namespace
	afNS = "http://authzforce.github.io/rest-api-model/xmlns/authz/5"

	// Atom namespace (used in link elements in AuthzForce responses)
	atomNS = "http://www.w3.org/2005/Atom"

	xsString = "http://www.w3.org/2001/XMLSchema#string"

	subjectCat  = "urn:oasis:names:tc:xacml:1.0:subject-category:access-subject"
	resourceCat = "urn:oasis:names:tc:xacml:3.0:attribute-category:resource"
	actionCat   = "urn:oasis:names:tc:xacml:3.0:attribute-category:action"

	subjectID  = "urn:oasis:names:tc:xacml:1.0:subject:subject-id"
	resourceID = "urn:oasis:names:tc:xacml:1.0:resource:resource-id"
	actionID   = "urn:oasis:names:tc:xacml:1.0:action:action-id"
)

// Client is an AuthzForce CE REST API client.
type Client struct {
	base string
	http *http.Client
}

// New returns a Client pointing at baseURL (e.g. "http://authzforce:8080/authzforce-ce").
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// EnsureDomain returns the domain ID for the given externalID, creating the
// domain if it does not exist. The caller should persist the returned ID and
// pass it to SetPolicy and Decide.
func (c *Client) EnsureDomain(externalID string) (string, error) {
	// Look up by externalID first.
	id, err := c.findDomain(externalID)
	if err != nil {
		return "", fmt.Errorf("findDomain: %w", err)
	}
	if id != "" {
		return id, nil
	}

	// Create the domain.
	body := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<ns2:domainProperties xmlns:ns2=%q><externalId>%s</externalId></ns2:domainProperties>`,
		afNS, externalID,
	)
	req, err := http.NewRequest(http.MethodPost, c.base+"/domains", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("create domain: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create domain returned %d: %s", resp.StatusCode, b)
	}

	// Parse href="/authzforce-ce/domains/{UUID}" from the link element.
	return extractHrefID(b)
}

// findDomain returns the domain ID matching externalID, or "" if not found.
func (c *Client) findDomain(externalID string) (string, error) {
	url := fmt.Sprintf("%s/domains?externalId=%s", c.base, externalID)
	resp, err := c.http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list domains returned %d: %s", resp.StatusCode, b)
	}
	return extractHrefID(b)
}

// extractHrefID finds the first href="…" or href='…' attribute in body and
// returns its last path segment. Returns "" if none is found.
// This handles both bare <link href="…"/> and envelope <resources><link href="…"/></resources>.
func extractHrefID(body []byte) (string, error) {
	s := string(body)
	for _, prefix := range []string{`href="`, `href='`} {
		idx := strings.Index(s, prefix)
		if idx < 0 {
			continue
		}
		rest := s[idx+len(prefix):]
		end := strings.IndexAny(rest, `"'`)
		if end < 0 {
			continue
		}
		return pathLast(rest[:end]), nil
	}
	return "", nil
}

func pathLast(s string) string {
	s = strings.TrimRight(s, "/")
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return s
	}
	return s[i+1:]
}

// SetPolicy uploads or replaces a XACML 3.0 PolicySet in the domain PAP.
// After uploading, it updates the domain PDP root policy reference to point
// at policySetID:version so decisions use the new policy immediately.
func (c *Client) SetPolicy(domainID, policyXML, policySetID, version string) error {
	// Upload the policy.
	url := fmt.Sprintf("%s/domains/%s/pap/policies", c.base, domainID)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(policyXML))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload policy: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("upload policy returned %d: %s", resp.StatusCode, b)
	}

	// Point the PDP at this policy version.
	return c.setRootPolicy(domainID, policySetID, version)
}

func (c *Client) setRootPolicy(domainID, policySetID, version string) error {
	ref := policySetID + ":" + version
	body := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<ns2:pdpPropertiesUpdate xmlns:ns2=%q>`+
			`<rootPolicyRefExpression>%s</rootPolicyRefExpression>`+
			`</ns2:pdpPropertiesUpdate>`,
		afNS, ref,
	)
	url := fmt.Sprintf("%s/domains/%s/pap/pdp.properties", c.base, domainID)
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("set root policy: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("set root policy returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

// Decide evaluates an access-control request against the domain PDP.
// Returns true for Permit, false for Deny/Indeterminate/NotApplicable.
func (c *Client) Decide(domainID, subject, resource, action string) (bool, error) {
	reqXML := buildXACMLRequest(subject, resource, action)
	url := fmt.Sprintf("%s/domains/%s/pdp", c.base, domainID)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(reqXML)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("PDP evaluate: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("PDP evaluate returned %d: %s", resp.StatusCode, b)
	}

	decision, err := parseDecision(b)
	if err != nil {
		return false, err
	}
	return decision == "Permit", nil
}

// buildXACMLRequest returns a XACML 3.0 Request XML for the given attributes.
func buildXACMLRequest(subject, resource, action string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<Request xmlns=%q CombinedDecision="false" ReturnPolicyIdList="false">`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute></Attributes>`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute></Attributes>`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute></Attributes>`+
			`</Request>`,
		xacmlNS,
		subjectCat, subjectID, xsString, subject,
		resourceCat, resourceID, xsString, resource,
		actionCat, actionID, xsString, action,
	)
}

// parseDecision extracts the Decision element value from a XACML Response.
func parseDecision(body []byte) (string, error) {
	type statusCode struct {
		Value string `xml:"Value,attr"`
	}
	type status struct {
		Code statusCode `xml:"StatusCode"`
	}
	type result struct {
		Decision string `xml:"Decision"`
		Status   status `xml:"Status"`
	}
	type response struct {
		Results []result `xml:"Result"`
	}

	var r response
	if err := xml.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("parse XACML response: %w", err)
	}
	if len(r.Results) == 0 {
		return "", fmt.Errorf("no Result in XACML response")
	}
	return r.Results[0].Decision, nil
}

// BuildPolicy generates a XACML 3.0 PolicySet XML from a list of grants.
// Each grant is (consumerSystemName, serviceDefinition).
// The PolicySet uses deny-unless-permit combining: any consumer with a
// matching grant is Permitted; all others are Denied.
func BuildPolicy(policySetID, version string, grants []Grant) string {
	var policies strings.Builder
	for _, g := range grants {
		policies.WriteString(buildGrantPolicy(g.Consumer, g.Service))
	}

	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<PolicySet xmlns=%q`+
			` PolicySetId=%q`+
			` Version=%q`+
			` PolicyCombiningAlgId="urn:oasis:names:tc:xacml:3.0:policy-combining-algorithm:deny-unless-permit">`+
			`<Description>Arrowhead experiment-5 unified telemetry access policy. `+
			`Generated from ConsumerAuthorization grants. Version %s.</Description>`+
			`<Target/>`+
			`%s`+
			`</PolicySet>`,
		xacmlNS, policySetID, version, version,
		policies.String(),
	)
}

// buildGrantPolicy generates one XACML Policy element permitting the given
// consumer to perform action "subscribe" on the given service resource.
func buildGrantPolicy(consumer, service string) string {
	policyID := fmt.Sprintf("urn:arrowhead:grant:%s:%s", consumer, service)
	return fmt.Sprintf(
		`<Policy PolicyId=%q Version="1.0"`+
			` RuleCombiningAlgId="urn:oasis:names:tc:xacml:3.0:rule-combining-algorithm:deny-unless-permit">`+
			`<Target><AnyOf><AllOf>`+
			`<Match MatchId="urn:oasis:names:tc:xacml:1.0:function:string-equal">`+
			`<AttributeValue DataType=%q>%s</AttributeValue>`+
			`<AttributeDesignator MustBePresent="true" Category=%q AttributeId=%q DataType=%q/>`+
			`</Match>`+
			`<Match MatchId="urn:oasis:names:tc:xacml:1.0:function:string-equal">`+
			`<AttributeValue DataType=%q>%s</AttributeValue>`+
			`<AttributeDesignator MustBePresent="true" Category=%q AttributeId=%q DataType=%q/>`+
			`</Match>`+
			`</AllOf></AnyOf></Target>`+
			`<Rule RuleId="permit" Effect="Permit"/>`+
			`</Policy>`,
		policyID,
		xsString, consumer, subjectCat, subjectID, xsString,
		xsString, service, resourceCat, resourceID, xsString,
	)
}

// Grant is a (consumer, service) pair derived from a ConsumerAuthorization rule.
type Grant struct {
	Consumer string
	Service  string
}
