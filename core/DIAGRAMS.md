# Arrowhead Core — Diagrams

---

## System Architecture

Core systems are shown in **blue**. Example application services (not part of the core) are shown in **green**. The browser dashboard is shown in **amber**.

```mermaid
graph TB
    classDef core    fill:#1e3a6e,color:#dce6f7,stroke:#3a6abf,stroke-width:2px
    classDef svc     fill:#1a4a26,color:#d4edda,stroke:#3a8a50,stroke-width:2px
    classDef ui      fill:#6b3a0a,color:#f7e6ce,stroke:#b06020,stroke-width:1px,stroke-dasharray:5 5

    subgraph CORE["Core Systems"]
        direction TB
        SR["ServiceRegistry\n:8080"]:::core
        AUTH["Authentication\n:8081"]:::core
        CA["ConsumerAuthorization\n:8082"]:::core
        DO["DynamicOrchestration\n:8083"]:::core
        SS["SimpleStoreOrchestration\n:8084"]:::core
        FS["FlexibleStoreOrchestration\n:8085"]:::core
    end

    subgraph SERVICES["Example Application Services"]
        direction LR
        TEMP["TemperatureSensor\n(provider)"]:::svc
        HUMID["HumidityMonitor\n(provider)"]:::svc
        CTRL["EnvironmentController\n(consumer)"]:::svc
    end

    DASH["Browser Dashboard"]:::ui

    TEMP  -->|"POST /register"| SR
    HUMID -->|"POST /register"| SR

    CTRL  -->|"POST /orchestration/dynamic"| DO
    CTRL  -->|"POST /orchestration/simplestore"| SS
    CTRL  -->|"POST /orchestration/flexiblestore"| FS

    DO    -->|"POST /query"| SR
    DO    -.->|"POST /verify (optional)"| CA

    DASH  -->|HTTP| SR
    DASH  -->|HTTP| CA
    DASH  -->|HTTP| DO
    DASH  -->|HTTP| SS
    DASH  -->|HTTP| FS
```

---

## Sequence Diagrams

### 1. Service Registration

A provider system announces a service to the Service Registry.

```mermaid
sequenceDiagram
    participant P as TemperatureSensor<br/>(provider)
    participant SR as ServiceRegistry<br/>:8080

    P->>SR: POST /serviceregistry/register<br/>{serviceDefinition, providerSystem, serviceUri, interfaces, metadata}
    SR-->>P: 201 Created — ServiceInstance {id, ...}

    note over SR: Stored under key<br/>(serviceDefinition, systemName, address, port, version).<br/>Duplicate registration overwrites, same id.
```

---

### 2. Service Unregistration

A provider removes its registration when shutting down.

```mermaid
sequenceDiagram
    participant P as TemperatureSensor<br/>(provider)
    participant SR as ServiceRegistry<br/>:8080

    P->>SR: DELETE /serviceregistry/unregister<br/>{serviceDefinition, providerSystem, version}
    SR-->>P: 204 No Content
```

---

### 3. Dynamic Orchestration (without authorization)

The orchestrator queries the Service Registry in real time and returns all matching providers.

```mermaid
sequenceDiagram
    participant C as EnvironmentController<br/>(consumer)
    participant DO as DynamicOrchestration<br/>:8083
    participant SR as ServiceRegistry<br/>:8080

    C->>DO: POST /orchestration/dynamic<br/>{requesterSystem, requestedService}
    DO->>SR: POST /serviceregistry/query<br/>{serviceDefinition, interfaces, metadata}
    SR-->>DO: 200 OK — {serviceQueryData: [...]}
    DO-->>C: 200 OK — {response: [{provider, service}, ...]}
    C->>C: pick provider, call service directly
```

---

### 4. Dynamic Orchestration (with authorization check)

When `ENABLE_AUTH=true`, the orchestrator additionally verifies that the consumer is authorized to access each candidate provider.

```mermaid
sequenceDiagram
    participant C as EnvironmentController<br/>(consumer)
    participant DO as DynamicOrchestration<br/>:8083
    participant SR as ServiceRegistry<br/>:8080
    participant CA as ConsumerAuthorization<br/>:8082

    C->>DO: POST /orchestration/dynamic<br/>{requesterSystem, requestedService}
    DO->>SR: POST /serviceregistry/query
    SR-->>DO: 200 OK — [TemperatureSensor, HumidityMonitor]

    loop for each candidate provider
        DO->>CA: POST /authorization/verify<br/>{consumerSystemName, providerSystemName, serviceDefinition}
        CA-->>DO: {authorized: true/false}
    end

    note over DO: Removes unauthorized providers from results.

    DO-->>C: 200 OK — {response: [authorized providers only]}
    C->>C: pick provider, call service directly
```

---

### 5. Authorization Rule Management

An administrator grants and later revokes a consumer→provider authorization rule.

```mermaid
sequenceDiagram
    participant A as Administrator
    participant CA as ConsumerAuthorization<br/>:8082
    participant DO as DynamicOrchestration<br/>:8083

    A->>CA: POST /authorization/grant<br/>{consumerSystemName, providerSystemName, serviceDefinition}
    CA-->>A: 201 Created — AuthRule {id: 7, ...}

    note over CA: Rule stored. DynamicOrchestration<br/>will now allow this pair.

    A->>CA: GET /authorization/lookup?consumer=EnvironmentController
    CA-->>A: 200 OK — {rules: [{id:7, ...}], count: 1}

    A->>CA: DELETE /authorization/revoke/7
    CA-->>A: 204 No Content

    note over DO: Next orchestration call for this<br/>consumer+provider pair will be rejected.
```

---

### 6. SimpleStore Orchestration

An administrator pre-configures a fixed routing rule. The consumer uses the rule without any SR lookup.

```mermaid
sequenceDiagram
    participant A as Administrator
    participant SS as SimpleStoreOrchestration<br/>:8084
    participant C as EnvironmentController<br/>(consumer)
    participant P as TemperatureSensor<br/>(provider)

    rect rgb(220, 240, 220)
        note over A,SS: Setup (one-time)
        A->>SS: POST /orchestration/simplestore/rules<br/>{consumerSystemName, serviceDefinition, provider, serviceUri, interfaces}
        SS-->>A: 201 Created — StoreRule {id: 3, ...}
    end

    rect rgb(210, 225, 245)
        note over C,P: Runtime
        C->>SS: POST /orchestration/simplestore<br/>{requesterSystem, requestedService}
        SS-->>C: 200 OK — {response: [{provider, service}]}
        C->>P: call service at provider address:port + serviceUri
    end
```

---

### 7. FlexibleStore Orchestration

Like SimpleStore but supports multiple rules per consumer+service, matched by metadata filter and ordered by priority.

```mermaid
sequenceDiagram
    participant A as Administrator
    participant FS as FlexibleStoreOrchestration<br/>:8085
    participant C as EnvironmentController<br/>(consumer)

    rect rgb(220, 240, 220)
        note over A,FS: Setup — two rules for same service
        A->>FS: POST /orchestration/flexiblestore/rules<br/>{priority:1, metadataFilter:{region:"eu"}, provider: EUSensor, ...}
        FS-->>A: 201 Created — FlexibleRule {id:1}

        A->>FS: POST /orchestration/flexiblestore/rules<br/>{priority:2, metadataFilter:{}, provider: GlobalSensor, ...}
        FS-->>A: 201 Created — FlexibleRule {id:2}
    end

    rect rgb(210, 225, 245)
        note over C,FS: Runtime — request with metadata {region:"eu"}
        C->>FS: POST /orchestration/flexiblestore<br/>{requestedService: {serviceDefinition: "temp", metadata: {region:"eu"}}}
        note over FS: Both rules match (id:2 filter is empty subset).<br/>id:1 wins — lower priority number = higher priority.
        FS-->>C: 200 OK — {response: [{provider: EUSensor, priority:1}, {provider: GlobalSensor, priority:2}]}
    end
```

---

### 8. Authentication Token Lifecycle

A system obtains an identity token, uses it, then logs out.

```mermaid
sequenceDiagram
    participant S as AnySystem
    participant AUTH as Authentication<br/>:8081

    S->>AUTH: POST /authentication/identity/login<br/>{systemName, credentials}
    AUTH-->>S: 201 Created — {token, systemName, expiresAt}

    note over S: Token attached to subsequent requests<br/>as: Authorization: Bearer <token>

    S->>AUTH: GET /authentication/identity/verify<br/>Authorization: Bearer <token>
    AUTH-->>S: 200 OK — {valid: true, systemName, expiresAt}

    note over S,AUTH: ... time passes, token still valid ...

    S->>AUTH: DELETE /authentication/identity/logout<br/>Authorization: Bearer <token>
    AUTH-->>S: 204 No Content

    S->>AUTH: GET /authentication/identity/verify<br/>Authorization: Bearer <token>
    AUTH-->>S: 200 OK — {valid: false}
```

---

### 9. Full End-to-End Flow

A complete interaction from provider startup through consumer service call, using Dynamic Orchestration with authorization.

```mermaid
sequenceDiagram
    participant P as TemperatureSensor<br/>(provider)
    participant SR as ServiceRegistry<br/>:8080
    participant CA as ConsumerAuthorization<br/>:8082
    participant DO as DynamicOrchestration<br/>:8083
    participant C as EnvironmentController<br/>(consumer)
    participant A as Administrator

    rect rgb(220, 240, 220)
        note over P,SR: 1 — Provider startup
        P->>SR: POST /serviceregistry/register
        SR-->>P: 201 ServiceInstance {id:1}
    end

    rect rgb(250, 235, 210)
        note over A,CA: 2 — Admin grants authorization
        A->>CA: POST /authorization/grant<br/>{consumer:"EnvironmentController", provider:"TemperatureSensor", service:"temperature"}
        CA-->>A: 201 AuthRule {id:1}
    end

    rect rgb(210, 225, 245)
        note over C,P: 3 — Consumer orchestrates and calls
        C->>DO: POST /orchestration/dynamic (ENABLE_AUTH=true)
        DO->>SR: POST /serviceregistry/query {serviceDefinition:"temperature"}
        SR-->>DO: [{id:1, provider: TemperatureSensor, ...}]
        DO->>CA: POST /authorization/verify
        CA-->>DO: {authorized: true, ruleId: 1}
        DO-->>C: {response: [{provider: TemperatureSensor, service: ...}]}
        C->>P: GET /temperature
        P-->>C: {value: 21.4, unit: "celsius"}
    end
```
