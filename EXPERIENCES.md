# EXPERIENCES — Implementation Mistakes and Lessons Learned

This file collects concrete mistakes made during implementation of ArrowheadCore
experiments, the symptoms that revealed them, and the guidance for avoiding them
in future iterations.

---

## EXP-001 — policy-sync hardcoded domain name (experiment-6, 2026-05-05)

### Symptom

All XACML authorization decisions return **Deny** for every consumer, including
those with active grants in ConsumerAuthorization.  `policy-sync /status` shows
`synced: true`, `grants: N` — so the policy was compiled and uploaded — but every
`/auth/check` call returns `"decision":"Deny"`.

```
demo-consumer-1: {"decision":"Deny","permit":false}   ← expected Permit
rest-consumer:   {"decision":"Deny","permit":false}   ← expected Permit
unauthorized:    {"decision":"Deny","permit":false}   ← correct
```

### Root Cause

`support/policy-sync/sync.go` contained a hardcoded domain name in `init()`:

```go
func (s *syncer) init() error {
    id, err := s.client.EnsureDomain("arrowhead-exp5")  // ← hardcoded
    ...
}
```

This worked for experiment-5, where all services used domain `"arrowhead-exp5"`.
In experiment-6, the environment variable `AUTHZFORCE_DOMAIN=arrowhead-exp6` was
set for the PEP services (`kafka-authz`, `rest-authz`), so they resolved and
queried the `"arrowhead-exp6"` domain — which had no policy uploaded to it.
Meanwhile policy-sync uploaded all policies to `"arrowhead-exp5"`.

The two sides were talking to different AuthzForce domains.

### Fix

1. `sync.go`: Change `init()` to accept the external domain ID as a parameter:
   ```go
   func (s *syncer) init(domainExtID string) error {
       id, err := s.client.EnsureDomain(domainExtID)
   ```

2. `main.go`: Read `AUTHZFORCE_DOMAIN` from the environment and pass it:
   ```go
   azDomainExt := envOr("AUTHZFORCE_DOMAIN", "arrowhead-exp5")
   ...
   s.init(azDomainExt)
   ```

The default `"arrowhead-exp5"` preserves backward compatibility with experiment-5.

### Guidance for Future Iterations

**When a service uses a configurable key (domain name, topic name, exchange name,
database name) that other services must agree on, the key must flow from an
environment variable — never be hardcoded — even if there is only one experiment
at the time of writing.**

Specific checks before shipping:

1. **Grep for hardcoded experiment names** in support modules:
   ```bash
   grep -r "exp5\|exp4\|experiment-5" support/ --include="*.go"
   ```
   Any match in non-comment code is a potential portability bug.

2. **Verify env-var symmetry**: for each shared key (e.g. `AUTHZFORCE_DOMAIN`),
   confirm that every service that reads it has the same default and that the
   docker-compose for each experiment sets it consistently.

3. **Cross-check the `init()` / startup path** of shared services against their
   env-var documentation.  The startup path is where hardcoded values are most
   dangerous because they may not fail immediately — they fail silently when the
   hardcoded value still resolves to something (an old domain, a stale topic).

4. **Smoke-test authorization explicitly** in `test-system.sh` before treating the
   stack as healthy.  The pattern `GET /health → 200` on a PEP does not prove that
   the PEP can produce Permit decisions; only an `/auth/check` or an actual data
   request can prove that.

---

## EXP-002 — test false-positive on error response body (experiment-6, 2026-05-05)

### Symptom

Test section 6 ("REST data access via rest-authz") reported **PASS** even though
the actual response was `{"error":"not authorized"}`.  The test was checking only
that the response body was non-null and non-empty:

```bash
if [ "$telemetry" != "null" ] && [ -n "$telemetry" ]; then
  pass "GET /telemetry/latest via rest-authz → data received"
```

An error JSON body satisfies both conditions and produces a spurious PASS.

### Fix

Add a negative check for the error key:

```bash
if [ "$telemetry" != "null" ] && [ -n "$telemetry" ] && ! echo "$telemetry" | grep -q '"error"'; then
  pass "..."
```

### Guidance for Future Iterations

**When testing a data-returning endpoint, check both the absence of an error key
and the presence of expected payload structure — not just that the body is non-empty.**

Common patterns:

```bash
# Check for expected field
echo "$body" | grep -q '"robotId"'

# Check response is not an error
! echo "$body" | grep -q '"error"'

# Check HTTP status code separately from body
http_code=$(curl -s -o /dev/null -w '%{http_code}' ...)
check_eq "endpoint → 200" "200" "$http_code"
```

Tests that only verify liveness (non-empty body, HTTP 200) can mask authorization
failures, proxy errors, and data-provider outages that return valid-looking JSON
error objects.

---

## EXP-003 — Code fix not deployed: Docker image not rebuilt (experiment-6, 2026-05-05)

### Symptom

After applying the EXP-001 fix (making policy-sync read `AUTHZFORCE_DOMAIN` from the
environment), re-running `bash test-system.sh` showed **identical auth failures** — all
authorized consumers still returned Deny.  The policy-sync `/status` showed
`synced:true, grants:7` but omitted the domain externalId, making it impossible to
tell which domain was actually being used.

```
FAIL  demo-consumer-1 → Permit (Kafka)
FAIL  rest-consumer → Permit (REST)
FAIL  analytics-consumer msgCount > 0
```

### Root Cause

The source-code fix was correct but the running Docker container still held the **old
binary** (compiled before the fix).  `docker compose up -d` (without `--build`) leaves
existing containers running unchanged.  The new `go` source was on disk but was never
compiled into the image.

The symptom is identical to the original EXP-001 bug because the old binary still
hardcodes `"arrowhead-exp5"`, so policy-sync uploads grants to the wrong domain and
all PDP decisions return Deny.

### Fix

1. Rebuild and restart: `docker compose up --build -d`
2. Added `domainExternalId` to policy-sync `/status` so the running domain can be
   verified without reading container logs:
   ```json
   {"domainExternalId":"arrowhead-exp6","grants":7,"synced":true,...}
   ```
3. Added a test check in section 2 of `test-system.sh`:
   ```bash
   grep -q '"domainExternalId":"arrowhead-exp6"'
   ```
   This check fails immediately with the old image, pinpointing the EXP-001/rebuild issue.

### Guidance for Future Iterations

**After any code change in `support/` Go modules, rebuild before testing:**

```bash
docker compose up --build -d
```

Never run `bash test-system.sh` immediately after editing Go source without first
rebuilding — the symptoms of a stale image are indistinguishable from an unfixed bug.

Specific checks:

1. **Verify `domainExternalId` in policy-sync /status** before blaming auth failures on
   XACML policy logic:
   ```bash
   curl -s http://localhost:3006/api/policy-sync/status | grep domainExternalId
   ```
   If the value is wrong (e.g. `arrowhead-exp5` instead of `arrowhead-exp6`), the
   image needs to be rebuilt, not the logic debugged.

2. **Use `--build` even when only one service changed** — Docker layer caching means
   `docker compose up -d` silently skips rebuilding services whose images already exist.

3. **Add `domainExternalId` to the `/status` endpoint** of any service that uses a
   configurable external ID, so the running configuration is always observable.

4. **Check the test section 2 output** (`policy-sync using domain arrowhead-exp6`)
   before investigating later failures — if section 2 fails on the domain check, all
   downstream auth failures are explained by that alone.

---

## Checklist — Before Adding a New Experiment

Use this before marking an experiment implementation complete:

- [ ] All shared support services read configurable keys from env vars (no experiment-N hardcoding in Go code)
- [ ] docker-compose env vars for each service are consistent (same domain, same topic names)
- [ ] `test-system.sh` includes explicit `/auth/check` tests for at least one Permit and one Deny case per PEP
- [ ] Data-endpoint tests verify payload content, not just HTTP 200 / non-empty body
- [ ] Revocation test waits at least `SYNC_INTERVAL + poll_interval` before asserting Deny
- [ ] Run `docker compose up --build -d` (with `--build`) before running `test-system.sh`
- [ ] policy-sync `/status` shows correct `domainExternalId` for this experiment before investigating auth failures
- [ ] `support/README.md` and `support/DIAGRAMS.md` updated for any new or modified support service
