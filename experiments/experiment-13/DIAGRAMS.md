# Experiment 13 — Architecture Diagrams

## System overview

```
┌────────────────────────────────────────────────────────────────────────────────┐
│                         Arrowhead Cloud (UC3)                                  │
│                                                                                │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐                       │
│  │ServiceRegistry│  │Authentication│   │ConsumerAuth  │  (AH5 spec —          │
│  │  TLS :8490   │  │  TLS :8491   │   │  TLS :8492   │   not called for authz)│
│  └──────────────┘  └──────────────┘   └──────────────┘                       │
│                                                                                │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │         DynamicOrch-XACML  :8083  (unchanged from exp-12)               │ │
│  │  gRPC Decide(subject, svc, provider, action=orchestrate) → authz-pdp    │ │
│  └──────────────────────────┬─────────────────────────────────────────────┘ │
│                             │ gRPC authorize.proto                            │
│                             ▼                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │  authz-pdp  :9550  (gRPC, authorize.proto — unchanged from exp-12)       │ │
│  └──────────────────────────┬─────────────────────────────────────────────┘ │
│                             │ XACML evaluate                                  │
│                             ▼                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐ │
│  │                         Policy Plane                                      │ │
│  │                                                                           │ │
│  │  ┌───────────────────────────────────┐  ┌────────────────────────────┐   │ │
│  │  │  PAP :9505                        │  │  PIP :9506                  │   │ │
│  │  │  action=orchestrate + provider    │  │  cert level registry        │   │ │
│  │  │  action=consume (no provider)     │  │  auto-populated via         │   │ │
│  │  └────────────────┬──────────────────┘  │  CertificateLifecycle gRPC  │   │ │
│  │                   │ XACML push          └────────────┬───────────────┘   │ │
│  │                   ▼                                  │ gRPC stream        │ │
│  │  ┌──────────────────────────────┐                   │ certlifecycle.proto │ │
│  │  │  AuthzForce PDP :8080        │                   ▼                    │ │
│  │  │  XACML PolicySet             │ ◀─── all 3 PEPs  ┌─────────────────┐  │ │
│  │  │  (urn:arrowhead:exp13)       │                   │  profile-ca     │  │ │
│  │  └──────────────────────────────┘                   │  :8087/:8088    │  │ │
│  └──────────────────────────────────────────────────────│  gRPC :8089    │  │ │
│                                                         └─────────────────┘  │
│  ┌────────────────────────────────────────────────────────────────────────┐   │
│  │                    Enforcement Plane (PEP) — NEW in exp-13             │   │
│  │                                                                        │   │
│  │  Before each AuthzForce call, PEP queries PIP:                        │   │
│  │  GET /pip/attributes/{cn} → {certLevel:"sy", valid:true}              │   │
│  │  Adds cert-level + cert-valid as XACML subject attributes.            │   │
│  │                                                                        │   │
│  │  ┌──────────────┐  ┌─────────────────┐  ┌──────────────────────────┐  │   │
│  │  │ kafka-authz  │  │ topic-auth-xacml│  │ pki-rest-authz            │  │   │
│  │  │ :9101        │  │ :9090           │  │ :9208 (mTLS) / :9209      │  │   │
│  │  │ Kafka mTLS   │  │ AMQP mTLS       │  │ REST mTLS                 │  │   │
│  │  │ principal=CN │  │ username=cert CN│  │ CN from cert handshake    │  │   │
│  │  └──────────────┘  └─────────────────┘  └──────────────────────────┘  │   │
│  └────────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────────┘
```

## CertificateLifecycle gRPC event flow

```
profile-ca.IssueSystemCert(cn="sp1", ou="sy")
  │ writes to cert registry
  │ emits CertEvent{cn=sp1, ou=sy, type=ISSUED, issuedAt=..., expiresAt=...}
  ▼
PIP gRPC subscriber (persistent stream)
  │ store.Register("sp1", "sy", valid=true)
  ▼
PEP: GET /pip/attributes/sp1
  → {systemName:"sp1", certLevel:"sy", valid:true}
PEP builds XACML request with:
  subject-id       = sp1
  cert-level       = sy     ← from PIP
  cert-valid       = true   ← from PIP
  resource-id      = telemetry
  action-id        = consume
AuthzForce → PERMIT (policy: subject=sp1, resource=telemetry, action=consume)

--- Revocation ---

curl -X DELETE http://profile-ca:8087/ca/certificates/sp1
  │ emits CertEvent{cn=sp1, ou=sy, type=REVOKED}
  ▼
PIP: store.Register("sp1", "sy", valid=false)
  ▼
Next PEP query: GET /pip/attributes/sp1 → {valid:false}
PEP XACML: cert-valid=false → AuthzForce DENY
```

## XACML request structure (exp-13 enforcement — with cert-level)

```xml
<Request>
  <Attributes Category="urn:oasis:names:tc:xacml:1.0:subject-category:access-subject">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:subject:subject-id">
      <AttributeValue>sp1</AttributeValue>          ← cert CN (verified by broker TLS)
    </Attribute>
    <Attribute AttributeId="urn:arrowhead:attribute:cert-level">
      <AttributeValue>sy</AttributeValue>           ← from PIP (NEW in exp-13)
    </Attribute>
    <Attribute AttributeId="urn:arrowhead:attribute:cert-valid">
      <AttributeValue>true</AttributeValue>         ← from PIP (NEW in exp-13)
    </Attribute>
  </Attributes>
  <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:resource:resource-id">
      <AttributeValue>telemetry-rest</AttributeValue>
    </Attribute>
  </Attributes>
  <Attributes Category="urn:oasis:names:tc:xacml:3.0:attribute-category:action">
    <Attribute AttributeId="urn:oasis:names:tc:xacml:1.0:action:action-id">
      <AttributeValue>consume</AttributeValue>
    </Attribute>
  </Attributes>
</Request>
```

## Identity source comparison across experiments

```
Exp-12:
  Row 1 (Kafka)  → subject = self-reported system name (NOT verified)
  Row 2 (AMQP)  → subject = self-reported username    (NOT verified)
  Row 3 (REST)  → subject = cert CN from mTLS         (verified by TLS)

Exp-13:
  Row 1 (Kafka)  → subject = cert CN  (ssl.client.auth=required, Kafka principal)
  Row 2 (AMQP)  → subject = cert CN  (rabbitmq_auth_mechanism_ssl)
  Row 3 (REST)  → subject = cert CN  (unchanged — already mTLS-verified)

  All rows: cert-level and cert-valid from PIP (auto-populated by profile-ca stream)
```

## Broker mTLS configuration changes

```
Kafka (exp-12):   KAFKA_SSL_CLIENT_AUTH=none
Kafka (exp-13):   KAFKA_SSL_CLIENT_AUTH=required

RabbitMQ (exp-12): verify_none, fail_if_no_peer_cert=false
RabbitMQ (exp-13): verify_peer, fail_if_no_peer_cert=true
                   rabbitmq_auth_mechanism_ssl enabled
                   auth_backends: certificate → http
```
