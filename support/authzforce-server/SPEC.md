# authzforce-server — HTTP API Specification

`authzforce-server` is a lightweight in-memory XACML 3.0 PDP/PAP that implements the
AuthzForce CE REST API. It replaces a full AuthzForce CE deployment in experiments,
providing domain management, policy upload, and authorization evaluation without
requiring a JVM.

Used by experiments 5 and 6. All three PEP services (`kafka-authz`, `rest-authz`,
`topic-auth-xacml`) and `policy-sync` talk to the same running instance via the
`authzforce` client library (`support/authzforce`).

---

## Environment Variables

| Variable | Default | Required | Description |
|---|---|:---:|---|
| `PORT` | `8080` | No | HTTP listening port |

All domain and policy state is stored in memory. There is no persistence — the server
starts with no domains and rebuilds state each time it restarts.

---

## Authorization Model

The server stores grants as `(consumer, service)` pairs, extracted from the
`PolicyId` attribute of uploaded XACML PolicySets. An authorization request is
evaluated by matching the `(subject, resource)` pair in the request against the
stored grant set for the target domain.

Policy combining algorithm: **deny-unless-permit**. Any request whose `(subject, resource)`
pair is not in the grant set receives `Deny`.

---

## Endpoints

### `GET /health`

Liveness probe. Always returns `200` once the process is running.

**Response `200 OK`**
```json
{"status":"ok"}
```

---

### `GET /authzforce-ce/domains?externalId={id}`

Look up a domain by its external identifier. Returns an XML resource list with a
`<link>` element whose `href` ends with the domain's internal UUID if found, or an
empty `<resources>` element if not found.

**Query Parameters**

| Parameter | Required | Description |
|---|:---:|---|
| `externalId` | Yes | The external identifier string to search for |

**Response `200 OK` — domain found**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<resources xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5">
  <link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/{uuid}"/>
</resources>
```

**Response `200 OK` — domain not found**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<resources xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5"/>
```

The `authzforce` client library (`EnsureDomain`) calls this endpoint first to check
whether a domain exists before creating one.

---

### `POST /authzforce-ce/domains`

Create a domain. If a domain with the same `externalId` already exists, returns the
existing domain's link without creating a duplicate (idempotent).

**Request Content-Type:** `application/xml;charset=UTF-8`

**Request Body**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ns2:domainProperties xmlns:ns2="http://authzforce.github.io/rest-api-model/xmlns/authz/5">
  <externalId>arrowhead-exp6</externalId>
</ns2:domainProperties>
```

| Element | Required | Description |
|---|:---:|---|
| `<externalId>` | Yes | External identifier; must be unique per experiment to avoid cross-domain grant leakage (see EXP-001) |

**Response `201 Created`**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/{uuid}"/>
```

The `{uuid}` in the response `href` is the internal domain identifier required by all
subsequent domain-scoped calls (`/pap/policies`, `/pap/pdp.properties`, `/pdp`).

---

### `PUT /authzforce-ce/domains/{id}/pap/policies`

Upload a XACML 3.0 PolicySet to a domain's Policy Administration Point. Replaces the
domain's entire grant set with the grants encoded in the PolicySet.

**URL Parameter:** `{id}` — domain UUID returned by `POST /authzforce-ce/domains`

**Request Content-Type:** `application/xml;charset=UTF-8`

**Request Body** — XACML 3.0 PolicySet with one `<Policy>` element per grant:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<PolicySet xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"
           PolicySetId="urn:arrowhead:exp6:telemetry"
           Version="1"
           PolicyCombiningAlgId="urn:oasis:names:tc:xacml:3.0:policy-combining-algorithm:deny-unless-permit">
  <Description>Generated from ConsumerAuthorization grants. Version 1.</Description>
  <Target/>
  <Policy PolicyId="urn:arrowhead:grant:rest-consumer:telemetry-rest" Version="1.0"
          RuleCombiningAlgId="urn:oasis:names:tc:xacml:3.0:rule-combining-algorithm:deny-unless-permit">
    <Target>
      <AnyOf>
        <AllOf>
          <Match MatchId="urn:oasis:names:tc:xacml:1.0:function:string-equal">
            <AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">rest-consumer</AttributeValue>
            <AttributeDesignator MustBePresent="true"
                                 Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject"
                                 AttributeId="urn:oasis:names:tc:xacml:1.0:subject:subject-id"
                                 DataType="http://www.w3.org/2001/XMLSchema#string"/>
          </Match>
          <Match MatchId="urn:oasis:names:tc:xacml:1.0:function:string-equal">
            <AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">telemetry-rest</AttributeValue>
            <AttributeDesignator MustBePresent="true"
                                 Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource"
                                 AttributeId="urn:oasis:names:tc:xacml:1.0:resource:resource-id"
                                 DataType="http://www.w3.org/2001/XMLSchema#string"/>
          </Match>
        </AllOf>
      </AnyOf>
    </Target>
    <Rule RuleId="permit" Effect="Permit"/>
  </Policy>
</PolicySet>
```

**Grant extraction:** The server parses `PolicyId` attributes using the pattern
`urn:arrowhead:grant:{consumer}:{service}` to build the domain's grant set.

**Response `200 OK`**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<link xmlns="http://www.w3.org/2005/Atom" href="/authzforce-ce/domains/{id}/pap/policies"/>
```

**Error Responses**

| Status | Condition |
|---|---|
| `404 Not Found` | Domain `{id}` does not exist |
| `405 Method Not Allowed` | Method other than `PUT` |

---

### `PUT /authzforce-ce/domains/{id}/pap/pdp.properties`

Set the root policy reference. Accepted for AuthzForce CE API compatibility but is a
no-op — the PDP always evaluates against the policy most recently uploaded via
`PUT /pap/policies`.

**URL Parameter:** `{id}` — domain UUID

**Request Content-Type:** `application/xml;charset=UTF-8`

**Request Body**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ns2:pdpPropertiesUpdate xmlns:ns2="http://authzforce.github.io/rest-api-model/xmlns/authz/5">
  <rootPolicyRefExpression>urn:arrowhead:exp6:telemetry:1</rootPolicyRefExpression>
</ns2:pdpPropertiesUpdate>
```

**Response `200 OK`**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<pdpProperties xmlns="http://authzforce.github.io/rest-api-model/xmlns/authz/5"/>
```

**Error Responses**

| Status | Condition |
|---|---|
| `404 Not Found` | Domain `{id}` does not exist |
| `405 Method Not Allowed` | Method other than `PUT` |

---

### `POST /authzforce-ce/domains/{id}/pdp`

Evaluate an authorization request (Policy Decision Point). Checks whether the
`(subject, resource)` pair in the request has a matching grant in the domain.

**URL Parameter:** `{id}` — domain UUID

**Request Content-Type:** `application/xml;charset=UTF-8`

**Request Body** — XACML 3.0 Request (generated by `authzforce.buildXACMLRequest`):
```xml
<?xml version="1.0" encoding="UTF-8"?>
<Request xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"
         CombinedDecision="false" ReturnPolicyIdList="false">
  <Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:subject:subject-id"
               IncludeInResult="false">
      <AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">rest-consumer</AttributeValue>
    </Attribute>
  </Attributes>
  <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:resource:resource-id"
               IncludeInResult="false">
      <AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">telemetry-rest</AttributeValue>
    </Attribute>
  </Attributes>
  <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:action:action-id"
               IncludeInResult="false">
      <AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">invoke</AttributeValue>
    </Attribute>
  </Attributes>
</Request>
```

The first two `<AttributeValue>` elements in document order are extracted as `subject`
and `resource` respectively. The `action` value is parsed but not used in the decision.

**Response `200 OK` — Permit**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
  <Result>
    <Decision>Permit</Decision>
  </Result>
</Response>
```

**Response `200 OK` — Deny**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
  <Result>
    <Decision>Deny</Decision>
  </Result>
</Response>
```

**Decision logic:** Returns `Permit` if and only if the grant pair `{subject, resource}`
exists in the domain's current grant set (i.e., was present in the last uploaded PolicySet).
All other requests receive `Deny`.

**Error Responses**

| Status | Condition |
|---|---|
| `404 Not Found` | Domain `{id}` does not exist |
| `405 Method Not Allowed` | Method other than `POST` |

---

## Client Library (`support/authzforce`)

The `authzforce` Go library wraps the server API. All four PEP/PAP services
(`kafka-authz`, `policy-sync`, `rest-authz`, `topic-auth-xacml`) use it exclusively —
they do not call the server directly.

### Key functions

**`EnsureDomain(url, externalID string) (domainID string, error)`**

Finds or creates a domain:
1. `GET /authzforce-ce/domains?externalId={externalID}` — returns UUID if found.
2. If not found, `POST /authzforce-ce/domains` with `<externalId>` body.
3. Parses the `href` of the returned `<link>` to extract the UUID.

Called at startup by every service that uses AuthzForce. Safe to call multiple times
with the same `externalID` — idempotent.

**`SetPolicy(url, domainID, policyXML, policySetID, version string) error`**

Uploads a policy to the PAP:
1. `PUT /authzforce-ce/domains/{domainID}/pap/policies` with XACML PolicySet XML.
2. `PUT /authzforce-ce/domains/{domainID}/pap/pdp.properties` with `rootPolicyRefExpression`.

**`Decide(url, domainID, subject, resource, action string) (permit bool, error)`**

Evaluates one authorization request:
1. Builds XACML 3.0 Request XML.
2. `POST /authzforce-ce/domains/{domainID}/pdp`.
3. Returns `true` on `Permit`, `false` on `Deny`.

**`BuildPolicy(policySetID, version string, grants []Grant) string`**

Generates a XACML 3.0 PolicySet XML string from a slice of `{Consumer, Service}` grants.
Each grant becomes one `<Policy>` element with `PolicyId="urn:arrowhead:grant:{Consumer}:{Service}"`.

---

## AUTHZFORCE_DOMAIN Invariant

All services that share an AuthzForce instance **must** use the same `AUTHZFORCE_DOMAIN`
value. A mismatch causes every authorization check to return `Deny` silently, because
each service creates or connects to a different domain and their policy sets are never
in sync.

```yaml
# All four services in the same experiment must have identical values:
AUTHZFORCE_URL:    http://authzforce:8080/authzforce-ce
AUTHZFORCE_DOMAIN: arrowhead-exp6   # change per experiment; never reuse across experiments
```

Each experiment should use a unique domain name (e.g. `arrowhead-exp7` for experiment-7)
to prevent accidental cross-experiment state pollution if two stacks share a network.

See `EXPERIENCES.md` EXP-001 for the original incident and diagnosis.

---

## Deployment

**Docker Compose health check:**
```yaml
authzforce:
  healthcheck:
    test: ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
    interval: 5s
    timeout: 3s
    retries: 10
```

**Exposed port:** Internal `8080` is mapped to `8186` in experiment-6
(`ports: ["8186:8080"]`). All inter-service traffic uses the internal port.

**Startup dependency:** `policy-sync` must start after `authzforce` is healthy and
call `EnsureDomain` before any PEP service begins evaluating requests. The Docker
Compose `depends_on` with `condition: service_healthy` enforces this ordering.
