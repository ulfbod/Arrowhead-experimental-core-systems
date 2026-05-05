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

## EXP-004 — grep `^` line anchor fails on SSE output stored in a bash variable (experiment-6, 2026-05-05)

### Symptom

Section 10 of `test-system.sh` (kafka-authz SSE stream) showed:

```
  first 300 chars: data: {"robotId":"robot-2",...}

data: {"robotId":"robot-3",...
  PASS  SSE test-probe: not denied (403)
  FAIL  SSE test-probe: data lines received from Kafka
         expected: data: {...}
         actual:   data: {"robotId":"robot-2",...}
```

The `actual` field in the FAIL message clearly shows `data: {` at the start of the output, yet
the check using `echo "$sse" | grep -q '^data:'` returned false.

### Root Cause

When SSE output is captured into a bash variable with `sse=$(timeout 4 curl -sN ...)` and then
piped back through `echo "$sse" | grep`, line-ending differences between the HTTP layer and the
shell variable can cause the `^` (start-of-line) anchor to fail.  HTTP responses over a Go
`net/http` server may carry CRLF (`\r\n`) chunk boundaries that survive into the shell variable
in environments such as WSL2; grep splits on `\n` but the `^` anchor then sees lines like
`data: {...}\r` — which start with `d` — yet the `^` interaction with `\r` can be
implementation-specific.  Anchor-free patterns such as `'not authorized'` work correctly
because they do not depend on line-start positioning.

The key observation: a grep pattern without `^` (e.g. `grep -q 'not authorized'`) succeeded on
the same `$sse` variable while `grep -q '^data:'` failed, even though the preview confirmed the
content started with `data:`.

### Fix

Replace the anchored pattern with a content-specific substring match:

```bash
# BEFORE (fails in some environments):
if echo "$sse" | grep -q '^data:'; then

# AFTER (reliable):
if echo "$sse" | grep -q 'data: {'; then
```

Using `'data: {'` (no `^`, includes the space and opening brace) is more specific than
bare `'^data:'` and not susceptible to line-anchor ambiguity.  It correctly matches SSE
lines of the form `data: {"robotId":...}`.

### Guidance for Future Iterations

**When grepping multi-line content captured in a bash variable via `$(...)`, avoid the `^`
and `$` anchors — use content-specific substrings instead.**

Common patterns for SSE / streaming test checks:

```bash
# Good — content-specific substring, no anchoring:
echo "$sse" | grep -q 'data: {'

# Good — bash built-in glob, checks the whole variable:
[[ "$sse" == *'data: {'* ]]

# Risky — ^ anchor may not work reliably with CRLF in piped shell variables:
echo "$sse" | grep -q '^data:'
```

The anchor-free `grep -q 'not authorized'` pattern used elsewhere in the test
file (for error detection) is a safe model to follow.

---

## EXP-005 — Escaped double quotes inside Mermaid edge label strings (experiment-6, 2026-05-05)

### Symptom

The Component Diagram in `experiments/experiment-6/DIAGRAMS.md` failed to render in
GitHub / VS Code with a parse error:

```
Parse error on line 66:
...{syncInterval:\"Ns\"}"| PS
...got 'STR'
```

The diagram source contained the following edge label:

```
DB  -.->|"POST /config\n{syncInterval:\"Ns\"}"| PS
```

### Root Cause

Mermaid does not support `\"` (backslash-escaped double quotes) inside double-quoted
`|"..."|` edge label strings.  The parser interprets the `\"` as an unexpected token
and aborts with a `STR` error.

### Fix

Remove the inner quotes entirely, using a bare value instead:

```
# BEFORE (broken):
DB  -.->|"POST /config\n{syncInterval:\"Ns\"}"| PS

# AFTER (fixed):
DB  -.->|"POST /config\n{syncInterval: Ns}"| PS
```

If the literal double-quote character is needed in a label, restructure the edge to
use a node label instead, or omit the quotes from the value.

### Guidance for Future Iterations

**Mermaid edge labels (`|"..."|`) do not support `\"` escape sequences — avoid nested
double quotes inside edge label strings entirely.**

Specific checks before committing diagrams:

1. **Search for `\"` in all `.md` files containing Mermaid blocks:**
   ```bash
   grep -r '\\"' experiments/ support/ --include="*.md"
   ```
   Any match inside a Mermaid code fence is a likely parse error.

2. **Validate diagrams render locally** (VS Code Mermaid preview or `mmdc` CLI)
   before committing, especially after editing edge labels.

3. **Prefer plain ASCII labels** — avoid `{}`, `"`, `'`, or special characters in
   Mermaid edge labels unless you have verified they render correctly.

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
- [ ] SSE / streaming checks in `test-system.sh` use content-specific substrings (e.g. `'data: {'`), not `^` line anchors
- [ ] `support/README.md` and `support/DIAGRAMS.md` updated for any new or modified support service
- [ ] All Mermaid diagrams render without parse errors (no `\"` inside `|"..."|` edge label strings)
