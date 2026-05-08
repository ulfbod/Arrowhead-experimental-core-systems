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

### Fix (first attempt — still unreliable, see EXP-006)

Replaced the anchored pattern with an anchor-free substring:

```bash
# BEFORE:
if echo "$sse" | grep -q '^data:'; then

# FIRST FIX (still failed — see EXP-006):
if echo "$sse" | grep -q 'data: {'; then
```

### Guidance for Future Iterations

**When grepping multi-line content captured in a bash variable via `$(...)`, avoid the `^`
and `$` anchors — use content-specific substrings instead.  Prefer bash `[[ ... ]]`
over `echo | grep` — see EXP-006 for why `echo ... | grep` is itself unreliable for
SSE variables.**

---

## EXP-006 — `echo "$var" | grep -q` unreliable for SSE bash variables (experiment-6, 2026-05-05)

### Symptom

After the EXP-004 fix changed `grep -q '^data:'` to the anchor-free `grep -q 'data: {'`,
section 10 of `test-system.sh` still reported **FAIL**, even though the `actual:` field in
the failure message — and the preview `${sse:0:300}` — clearly showed `data: {` present in
`$sse`:

```
  first 300 chars: data: {"robotId":"robot-1",...}

data: {"robotId":"robot-2",...
  PASS  SSE test-probe: not denied (403)
  FAIL  SSE test-probe: data lines received from Kafka
         expected: data: {...}
         actual:   data: {"robotId":"robot-1",...}
```

The EXP-004 fix that removed the `^` anchor was not sufficient.

### Root Cause

`echo "$sse" | grep -q 'data: {'` is unreliable for SSE output captured via `$(...)` even
without line anchors.  The exact mechanism could not be determined analytically; plausible
contributors include:

- `echo` interpreting or dropping certain byte sequences in the variable value
- Buffering or piping behaviour differences in bash on WSL2 / Linux containers
- Interaction between the shell built-in `echo` and the grep process when the variable
  contains CRLF, embedded null bytes, or other non-printable characters from the SSE stream

Critically: `[[ "$var" == *"pattern"* ]]` uses bash's built-in string matching and never
invokes an external process, so it is immune to all piping and echo-interpretation issues.
It also avoids spawning a subshell.

### Fix

Replace `echo "$sse" | grep -q '...'` with bash's built-in glob test:

```bash
# BEFORE (unreliable — EXP-004 first attempt):
if echo "$sse" | grep -q 'data: {'; then

# AFTER (reliable):
if [[ "$sse" == *"data: {"* ]]; then
```

Applied to section 10 of `experiments/experiment-6/test-system.sh`.

### Guidance for Future Iterations

**For substring checks on bash variables containing SSE or streaming output, always use
`[[ "$var" == *"substring"* ]]` instead of `echo "$var" | grep -q`.**

Pattern summary:

```bash
# Best — bash built-in, no subprocess, immune to echo/pipe issues:
[[ "$sse" == *"data: {"* ]]

# Acceptable for error-string checks (short, ASCII, no special chars):
echo "$sse" | grep -q 'not authorized'

# Unreliable for SSE variable content (EXP-004 + EXP-006):
echo "$sse" | grep -q 'data: {'
echo "$sse" | grep -q '^data:'
```

---

## EXP-007 — kafka-go consumer group reader fails silently when topic does not exist at startup (experiment-6, 2026-05-05)

### Symptom

Section 6 of `test-system.sh` (REST data access via rest-authz) polled
`http://localhost:9093/telemetry/latest` 12 times (60 s total) and received `null` every
time:

```
  telemetry/latest (first 200 chars): null
  FAIL  GET /telemetry/latest via rest-authz → data received
         expected: non-null JSON without error
         actual:   null
```

This happened despite robot-fleet publishing to Kafka at 30 messages/s (confirmed by
section 10 SSE stream showing live telemetry data) and rest-authz correctly returning
403 for unauthorized requests (confirming the auth path worked).  data-provider's
`/telemetry/latest` returns the literal string `null` when no Kafka message has been
stored, indicating its Kafka consumer received zero messages.

### Root Cause

`data-provider` used a `kafka-go` consumer group reader:

```go
r := kafka.NewReader(kafka.ReaderConfig{
    Brokers:        brokers,
    Topic:          topic,
    GroupID:        "data-provider",
    StartOffset:    kafka.LastOffset,
    ...
})
```

`data-provider` starts before `robot-fleet` (it only depends on Kafka being healthy, not
on robot-fleet).  When the topic `arrowhead.telemetry` does not yet exist at the time
the consumer group reader initialises, the Kafka group coordinator may fail to assign
the partition, leaving the reader in a state where it never receives messages — even
after the topic is subsequently created and robot-fleet begins publishing.

`kafka-authz`, which also consumes `arrowhead.telemetry` and works correctly, uses a
**partition-level reader** (no `GroupID`) that bypasses the group coordinator entirely.

### Fix

Switch `data-provider` from a consumer group reader to a partition reader (same pattern
as `kafka-authz`):

```go
// BEFORE:
r := kafka.NewReader(kafka.ReaderConfig{
    Brokers:        brokers,
    Topic:          topic,
    GroupID:        "data-provider",
    CommitInterval: time.Second,
    StartOffset:    kafka.LastOffset,
    ...
})

// AFTER:
r := kafka.NewReader(kafka.ReaderConfig{
    Brokers:     brokers,
    Topic:       topic,
    Partition:   0,
    MaxWait:     500 * time.Millisecond,
    StartOffset: kafka.LastOffset,
    ...
})
```

Applied to `experiments/experiment-6/services/data-provider/main.go`.

### Guidance for Future Iterations

**Use a partition-level reader (no `GroupID`) for any Kafka consumer that starts before
the producing service and needs to read from a single-partition topic.**

Consumer group readers add coordination overhead that can fail silently when the topic
does not yet exist.  Partition readers start cleanly even on a non-existent topic and
recover automatically once the topic is created.

Specific checks:

1. If a new Kafka consumer service does NOT need cross-instance coordination (i.e., it is
   a single-instance reader maintaining a latest-state cache), prefer `Partition: 0` over
   `GroupID`.

2. If a consumer group is required (e.g., for horizontal scaling), ensure the producing
   service starts and publishes at least one message before the consumer's healthcheck
   passes — use `depends_on: robot-fleet: condition: service_healthy`.

3. Smoke-test data-provider directly (before section 6): `curl http://localhost:9094/stats`
   should show `"msgCount": N > 0` if Kafka is delivering messages.

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

## EXP-008 — Test files included in production TypeScript build break `npm run build` (experiment-6, 2026-05-06)

### Symptom

Docker image build for the experiment-6 dashboard failed during `npm run build`:

```
src/api.test.ts(24,17): error TS2322: Type 'VitestUtils' is not assignable to type 'Awaitable<void>'.
failed to solve: process "/bin/sh -c npm run build" did not complete successfully: exit code: 2
```

The error appeared after adding unit tests for the new Kafka and REST tabs.
The production build had worked before; adding `*.test.ts` files to `src/` broke it.

### Root Cause

The production build script was `tsc && vite build`.  With `tsconfig.json` including
all files under `src/`, `tsc` compiled test files alongside application code.  Test
files import vitest-specific APIs (e.g. `vi.stubGlobal`) that return vitest types
(`VitestUtils`).  Those types are incompatible with the standard `Awaitable<void>`
return type that `afterEach` expects, causing the `TS2322` error.

The root issue is the lack of a build-specific tsconfig that excludes test files.
A single `tsconfig.json` covering both production and test code works in the IDE
but fails when `tsc` is run as a build gate in Docker.

### Fix

1. Create `tsconfig.app.json` — identical compiler options to `tsconfig.json` but
   with an `exclude` for all test files:

   ```json
   {
     "compilerOptions": { /* same as tsconfig.json */ },
     "include": ["src"],
     "exclude": ["src/**/*.test.ts", "src/**/*.test.tsx", "src/test"],
     "references": [{ "path": "./tsconfig.node.json" }]
   }
   ```

2. Update the build script in `package.json` to use the app-only tsconfig:

   ```json
   "build": "tsc -p tsconfig.app.json && vite build"
   ```

`tsconfig.json` is kept unchanged so the IDE (and `vitest`) continues to type-check
test files with full vitest type support.

### Guidance for Future Iterations

**Whenever unit tests are added to a Vite dashboard, ensure the project has a
separate `tsconfig.app.json` that excludes test files from the production `tsc` run.**

Specific checks:

1. **On any new dashboard project**, immediately create `tsconfig.app.json` and
   point the `build` script at it — do not wait until test files cause a breakage.

2. **The split tsconfig pattern**:
   - `tsconfig.json` — full `src/` include, used by the IDE and by `vitest`
   - `tsconfig.app.json` — same options, excludes `**/*.test.*` and `src/test/`, used by `npm run build`

3. **Verify the Docker build locally** after adding the first test file to a dashboard:
   ```bash
   docker compose build dashboard
   ```
   A type error here means `tsconfig.app.json` is missing or the build script still
   references `tsconfig.json`.

4. **Do not work around this by suppressing the type error in the test file** (e.g.
   using block-body `{ vi.stubGlobal(...) }` to avoid returning `VitestUtils`).
   That hides the symptom without fixing the structural problem — a future test
   importing other vitest APIs will trigger the same failure again.

---

## EXP-009 — ServiceRegistry query response uses `serviceQueryData`, not `serviceInstances` (experiment-6, 2026-05-06)

### Symptom

The system test section 11 check for the ServiceRegistry query endpoint failed:

```
SR query for telemetry-rest (first 200 chars): {"serviceQueryData":[],"unfilteredHits":2}
FAIL  POST /api/serviceregistry/query via nginx
       expected: "serviceInstances":[...]
       actual:   {"serviceQueryData":[],"unfilteredHits":2}
```

The dashboard's `RestView` also silently showed an empty table in all cases — the
`serviceQueryData` array was never read because the component accessed
`data.serviceInstances` which was always `undefined`.

### Root Cause

The dashboard types and component were written against an assumed response shape
(`serviceInstances` / `count`) without consulting `core/SPEC.md`.  The actual
ServiceRegistry query response (per spec) uses:

```json
{ "serviceQueryData": [ /* ServiceInstance[] */ ], "unfilteredHits": 0 }
```

The secondary finding: in this experiment `data-provider` does not self-register
with the ServiceRegistry, so `serviceQueryData` is always `[]` even after the fix.
The `unfilteredHits: 2` shows other services are registered, just not `telemetry-rest`.
This is expected behaviour for the current experiment design.

### Fix

1. `types.ts`: rename `serviceInstances → serviceQueryData`, `count → unfilteredHits`
2. `RestView.tsx`: access `data.serviceQueryData` and `data.unfilteredHits`; updated
   hint text to explain that data-provider does not self-register in this experiment
3. `api.test.ts`, `RestView.test.tsx`: update mock data to use correct field names
4. `test-system.sh`: check for `"serviceQueryData"` and `"unfilteredHits"` fields;
   remove the assertion that instances are present (data-provider doesn't register)

### Guidance for Future Iterations

**Always read `core/SPEC.md` before writing TypeScript types or test assertions for
any Arrowhead core system API response shape.  Do not infer field names from first
principles or REST conventions.**

Specific checks:

1. Before writing a new API type in `types.ts`, grep SPEC.md for the endpoint:
   ```bash
   grep -A 20 "serviceregistry/query" core/SPEC.md
   ```

2. If a UI section shows an unexpectedly empty list, check whether the field name
   in the component matches the actual JSON key — not just that the HTTP call
   returns 200.

3. When a system test asserts response body fields, assert the exact field names
   from the spec, not assumed REST-conventional names.

---

## EXP-010 — Docker COPY does not dereference relative symlinks (dashboard shared source, 2026-05-07)

### Symptom

Docker build for the experiment-5 and experiment-6 dashboards failed at `npm run build`
with a cascade of TypeScript `TS2307: Cannot find module` errors for every file that was
symlinked from `support/dashboard-shared/`:

```
src/App.tsx(2,32): error TS2307: Cannot find module './config/context'
src/App.tsx(3,29): error TS2307: Cannot find module './views/HealthView'
src/components/SystemHealthGrid.tsx(2,28): error TS2307: Cannot find module '../hooks/usePolling'
...
```

The COPY step itself completed without error; the failure only appeared when `tsc` ran.

### Root Cause

Ten source files in `dashboard/src/` are relative symlinks pointing to
`support/dashboard-shared/` (e.g. `../../../../support/dashboard-shared/main.tsx`).

Docker's `COPY` command copies relative symlinks **as-is** — it does not dereference
them. Inside the container the files landed at `/app/src/main.tsx` (a symlink), but
the symlink target `../../../../support/dashboard-shared/main.tsx`, resolved from
`/app/src/`, points to `/support/dashboard-shared/main.tsx` — a path that does not
exist in the container. TypeScript therefore could not find any of the shared modules.

The COPY step produced no error because Docker successfully copied the symlink entries
themselves; the failure was deferred to `tsc`, which tried to open the (dangling) files.

### Fix

The Dockerfile must explicitly copy the shared files and remove the dangling symlinks
before the build runs.  **Use `cp -rn` (no-clobber), not plain `cp -r`** — otherwise the
stubs in `support/dashboard-shared/components/` overwrite the real experiment-specific
implementations (see secondary finding below):

```dockerfile
COPY experiments/experiment-N/dashboard/ .
COPY support/dashboard-shared/ /dashboard-shared/
RUN find src -type l | while read link; do \
      rel="${link#src/}"; \
      rm "$link" && cp "/dashboard-shared/$rel" "$link"; \
    done
RUN npm run build
```

Step-by-step:
1. `COPY experiments/experiment-N/dashboard/ .` — copies the dashboard; symlinks arrive as dangling and real component files are present.
2. `COPY support/dashboard-shared/ /dashboard-shared/` — copies shared files to a scratch path.
3. The `while read` loop iterates over every symlink in `src/`, removes it, and replaces it with the real file from `/dashboard-shared/` at the same relative path. Real (non-symlink) files are never touched.
4. `npm run build` runs with all files in place.

**Why `cp -r /dashboard-shared/. src/` is wrong** (original approach, now removed):
`support/dashboard-shared/components/` contains stub implementations of `SystemHealthGrid`,
`GrantsPanel`, `PolicyProjectionPanel`, and `ConsumerStatsPanel` (each returns `null`) so
that the shared test suite can compile without the real experiment-specific implementations.
A blanket `cp -r` overwrites the real experiment components with these stubs — the build
succeeds silently, and those tabs render completely blank at runtime.

**Why `cp -rn` is also wrong**: Alpine Linux uses BusyBox `cp`, which does not support the
`-n` (no-clobber) flag reliably. The loop-based approach above is the portable, correct fix.

Applied to both `experiments/experiment-5/dockerfiles/dashboard.Dockerfile` and
`experiments/experiment-6/dockerfiles/dashboard.Dockerfile`.

### Guidance for Future Experiments

**When a dashboard's `src/` contains symlinks to files outside the dashboard directory,
the Dockerfile must resolve them explicitly — never rely on Docker to dereference
relative symlinks.**

Specific checks before shipping a dashboard Dockerfile:

1. **If any file in `src/` is a symlink**, add the three-step pattern above to the
   Dockerfile. The pattern is safe even if there are no symlinks: `find src -type l -delete`
   is a no-op when there are none.

2. **Verify symlinks are dangling inside the container** by adding a temporary diagnostic:
   ```dockerfile
   RUN find src -type l | while read f; do echo "$f -> $(readlink $f)"; done
   ```
   Any output confirms symlinks are present and shows where they point.

3. **Never rely on `docker compose build --no-cache` to reveal this problem** — the COPY
   step succeeds silently; the error only appears at `tsc` or `npm run build`. The
   symptom is indistinguishable from a missing file or a wrong import path.

4. **The `check-dashboard-shared.sh` script verifies symlinks on the host filesystem**
   (where they are valid). It cannot detect the Docker-side problem. The only way to
   test the Docker side is to run `docker compose build dashboard`.

---

## EXP-011 — Local `npm run build` fails with "Could not resolve './App'" due to Vite symlink resolution (experiment-6, 2026-05-07)

### Symptom

Running `npm run build` locally in `experiments/experiment-6/dashboard/` failed with:

```
x Build failed in 73ms
error during build:
Could not resolve "./App" from "../../../support/dashboard-shared/main.tsx"
```

Confusingly:
- `npm run typecheck` (`tsc --noEmit`) **passed** — the TypeScript compiler resolves symlinks correctly.
- `docker compose up --build` **passed** — the Docker build worked fine.
- The dashboard was **unreachable** in a browser at `http://localhost:3006/` because the Docker build was producing a stale image from a previous successful build (before the symlink issue was introduced), and the current Docker image was actually broken.

### Root Cause

Ten source files in `dashboard/src/` are symlinks to `support/dashboard-shared/` (e.g. `main.tsx → ../../../../support/dashboard-shared/main.tsx`).

Vite uses Rollup for bundling. Without explicit configuration, Rollup **follows symlinks to their real path** before resolving relative imports. So when it processes `src/main.tsx` (a symlink), it treats the file as if it lives at `support/dashboard-shared/main.tsx` and resolves the `./App` import relative to `support/dashboard-shared/` — a directory that does not contain `App.tsx`.

`tsc --noEmit` is unaffected because TypeScript resolves relative imports from the location of the symlink, not the real file.

The Docker build avoided this problem because `dashboard.Dockerfile` explicitly removes all symlinks and replaces them with real copies before running `npm run build`:
```dockerfile
RUN find src -type l -delete && cp -r /dashboard-shared/. src/
```
This means a locally broken build can produce a working Docker image — or vice versa — making the failure invisible unless `npm run build` is tested directly.

### Fix

Add `resolve: { preserveSymlinks: true }` to `vite.config.ts`:

```typescript
export default defineConfig({
  plugins: [react()],
  resolve: {
    // src/main.tsx (and nine other shared files) are symlinks to support/dashboard-shared/.
    // Without this, Rollup follows symlinks to the real path and resolves relative imports
    // from dashboard-shared/ — causing "Could not resolve './App'" at build time.
    // Docker builds are unaffected (Dockerfile removes symlinks before building).
    preserveSymlinks: true,
  },
  server: { ... },
  test: { ... },
})
```

This tells Rollup to resolve imports relative to the symlink's location, not its real path.

Applied to `experiments/experiment-6/dashboard/vite.config.ts`.

### Guidance for Future Experiments

**Any Vite project whose `src/` contains symlinks to files outside the project directory must set `resolve.preserveSymlinks: true` in `vite.config.ts`.**

Specific checks:

1. **After adding symlinks to `src/`**, always run both:
   ```bash
   npm run typecheck   # passes even without preserveSymlinks
   npm run build       # reveals the Rollup symlink issue
   ```
   A passing typecheck does NOT guarantee a passing build when symlinks are involved.

2. **The test-system.sh pre-flight** now verifies the main JS bundle URL returns HTTP 200 (not just that the HTML contains `<div id="root">`). This catches the case where Docker serves stale HTML referencing a non-existent bundle.

3. **When adding a new experiment dashboard that symlinks from `support/dashboard-shared/`**, add `resolve: { preserveSymlinks: true }` to `vite.config.ts` from the start — do not copy the config from experiment-5 which predates this fix.

4. **The three-step symlink-removal pattern in the Dockerfile** (EXP-010) and `preserveSymlinks: true` are complementary, not alternatives — the Dockerfile pattern keeps Docker builds correct; `preserveSymlinks` keeps local development builds correct.

---

## EXP-012 — Dashboard unreachable in Windows browser when using Docker Engine directly in WSL2 (experiment-6, 2026-05-08)

### Symptom

The dashboard is unreachable at `http://localhost:3006/` in a Windows browser, even though:
- `curl http://localhost:3006/` from within WSL2 returns HTTP 200.
- `bash test-system.sh` passes all tests (including the nginx pre-flight).
- The Docker containers are running and healthy.

### Root Cause

Docker Engine installed directly inside WSL2 (not via Docker Desktop) binds container ports to the WSL2 VM's network interface. From within WSL2, `localhost` resolves to the WSL2 VM, so `curl localhost:3006` works. From Windows, `localhost` resolves to the Windows loopback (`127.0.0.1`), which has no listener on port 3006 — the port was never forwarded out of the WSL2 VM.

Docker Desktop handles this automatically by registering a Windows-side port proxy for every exposed container port. Docker Engine in WSL2 does not.

### Fix (immediate)

Use the WSL2 VM IP address in the Windows browser:

```bash
# From a WSL2 terminal:
hostname -I | awk '{print $1}'
# Example output: 172.26.149.70
# Then open: http://172.26.149.70:3006/
```

The WSL2 IP changes on every `wsl --shutdown` / restart cycle.

### Fix (permanent)

Enable WSL2 mirrored networking so all WSL2 ports appear on Windows `localhost`.
Create (or edit) `C:\Users\<YourUsername>\.wslconfig` on the Windows filesystem:

```ini
[wsl2]
networkingMode=mirrored
```

Then shut down and restart WSL2:

```powershell
# In Windows PowerShell / cmd:
wsl --shutdown
# Re-open WSL2 terminal — now localhost:3006 works from Windows browsers
```

After this, `localhost:3006` in a Windows browser works the same as Docker Desktop.

### Guidance for Future Experiments

**Document the WSL2 networking caveat in every experiment README.** The `test-system.sh` cannot detect this problem because it runs curl from within WSL2 where localhost always resolves correctly.

Specific notes:

1. **All experiment READMEs should include the WSL2 note** alongside the `localhost:<port>` browser instructions.
2. **`test-system.sh` cannot verify Windows browser access** — the pre-flight only proves the server is reachable from within WSL2.
3. **Docker Desktop users are unaffected** — port forwarding is automatic and `localhost` works from Windows browsers.
4. **The `networkingMode=mirrored` setting** in `.wslconfig` is the cleanest permanent fix. It applies globally to all WSL2 port listeners, not just Docker containers.

---

## Checklist — Before Adding a New Experiment

Use this before marking an experiment implementation complete:

- [ ] `test-system.sh` sources `../test-lib.sh` (no inline helper declarations); uses `assert_http`, `assert_contains`, `assert_json_field`, `assert_json_value`, `assert_json_gt` for all assertions — no bare `echo "$x" | grep -q` patterns used as assertions
- [ ] `test-system.sh` has a `=== Pre-flight: smoke-check ===` section with `smoke_fail`/`smoke_http` helpers that exits immediately on any fundamental failure before application-level tests run
- [ ] All shared support services read configurable keys from env vars (no experiment-N hardcoding in Go code)
- [ ] docker-compose env vars for each service are consistent (same domain, same topic names)
- [ ] `test-system.sh` includes explicit `/auth/check` tests for at least one Permit and one Deny case per PEP
- [ ] Data-endpoint tests verify payload content, not just HTTP 200 / non-empty body
- [ ] Revocation test waits at least `SYNC_INTERVAL + poll_interval` before asserting Deny
- [ ] Run `docker compose up --build -d` (with `--build`) before running `test-system.sh`
- [ ] policy-sync `/status` shows correct `domainExternalId` for this experiment before investigating auth failures
- [ ] SSE / streaming checks in `test-system.sh` use `[[ "$var" == *"pattern"* ]]`, not `echo | grep` (EXP-004, EXP-006)
- [ ] Any Kafka consumer that starts before the producing service uses a partition reader (`Partition: 0`), not a consumer group reader (`GroupID`) (EXP-007)
- [ ] Smoke-check data-provider `/stats` directly (`msgCount > 0`) before investigating rest-authz failures
- [ ] `support/README.md` and `support/DIAGRAMS.md` updated for any new or modified support service
- [ ] All Mermaid diagrams render without parse errors (no `\"` inside `|"..."|` edge label strings)
- [ ] Dashboard `package.json` build script uses `tsc -p tsconfig.app.json` (not bare `tsc`) to exclude test files from production build (EXP-008)
- [ ] Dashboard TypeScript types for core system API responses match field names in `core/SPEC.md` exactly (e.g. `serviceQueryData`/`unfilteredHits`, not `serviceInstances`/`count`) (EXP-009)
- [ ] If any file in `dashboard/src/` is a symlink to `support/dashboard-shared/`, the dashboard Dockerfile uses the loop pattern: `find src -type l | while read link; do rel="${link#src/}"; rm "$link" && cp "/dashboard-shared/$rel" "$link"; done` — never use `cp -r /dashboard-shared/. src/` (overwrites real components with stubs) or `cp -rn` (not supported in Alpine BusyBox) (EXP-010)
- [ ] If any file in `dashboard/src/` is a symlink, `vite.config.ts` contains `resolve: { preserveSymlinks: true }` — run `npm run build` locally (not just `npm run typecheck`) to verify (EXP-011)
- [ ] README.md includes the WSL2 networking note alongside browser access instructions — users running Docker Engine directly in WSL2 must use the WSL2 IP (`hostname -I`) or enable `networkingMode=mirrored` in `.wslconfig` (EXP-012)
