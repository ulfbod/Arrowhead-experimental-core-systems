// xacml_enriched.go — local XACML helper that adds cert-level subject attributes.
//
// decideWithCertLevel builds a XACML 3.0 request with three subject attributes:
//   - subject-id      (standard)
//   - cert-level      (Arrowhead extension)
//   - cert-valid      (Arrowhead extension)
//
// It POSTs directly to AuthzForce without using the shared authzforce library.
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	xacmlNS     = "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"
	xsString    = "http://www.w3.org/2001/XMLSchema#string"
	xsBoolean   = "http://www.w3.org/2001/XMLSchema#boolean"
	subjectCat  = "urn:oasis:names:tc:xacml:1.0:subject-category:access-subject"
	resourceCat = "urn:oasis:names:tc:xacml:3.0:attribute-category:resource"
	actionCat   = "urn:oasis:names:tc:xacml:3.0:attribute-category:action"
	subjectID   = "urn:oasis:names:tc:xacml:1.0:subject:subject-id"
	resourceID  = "urn:oasis:names:tc:xacml:1.0:resource:resource-id"
	actionID    = "urn:oasis:names:tc:xacml:1.0:action:action-id"
	certLevelID = "urn:arrowhead:attribute:cert-level"
	certValidID = "urn:arrowhead:attribute:cert-valid"
)

var enrichedHTTPClient = &http.Client{Timeout: 10 * time.Second}

// buildEnrichedXACMLRequest constructs the XACML 3.0 XML with cert-level subject attrs.
func buildEnrichedXACMLRequest(subject, service, action, certLevel string, certValid bool) string {
	certValidStr := "false"
	if certValid {
		certValidStr = "true"
	}
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<Request xmlns=%q CombinedDecision="false" ReturnPolicyIdList="false">`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute>`+
			`</Attributes>`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute>`+
			`</Attributes>`+
			`<Attributes Category=%q>`+
			`<Attribute AttributeId=%q IncludeInResult="false">`+
			`<AttributeValue DataType=%q>%s</AttributeValue></Attribute>`+
			`</Attributes>`+
			`</Request>`,
		xacmlNS,
		subjectCat,
		subjectID, xsString, subject,
		certLevelID, xsString, certLevel,
		certValidID, xsBoolean, certValidStr,
		resourceCat,
		resourceID, xsString, service,
		actionCat,
		actionID, xsString, action,
	)
}

// decideWithCertLevel posts an enriched XACML request to AuthzForce and returns
// true for Permit, false for Deny/Indeterminate/NotApplicable.
func decideWithCertLevel(azURL, domainID, subject, service, action, certLevel string, certValid bool) (bool, error) {
	reqXML := buildEnrichedXACMLRequest(subject, service, action, certLevel, certValid)
	url := fmt.Sprintf("%s/domains/%s/pdp", azURL, domainID)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(reqXML)))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")
	req.Header.Set("Accept", "application/xml")

	resp, err := enrichedHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("PDP evaluate: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("PDP evaluate returned %d: %s", resp.StatusCode, b)
	}
	decision, err := parseXACMLDecision(b)
	if err != nil {
		return false, err
	}
	return decision == "Permit", nil
}

// parseXACMLDecision extracts the Decision element value from a XACML Response.
func parseXACMLDecision(body []byte) (string, error) {
	type result struct {
		Decision string `xml:"Decision"`
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
