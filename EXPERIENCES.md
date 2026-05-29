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

## EXP-013 — `apt-get` used in Kafka TLS Dockerfile, but cp-kafka image is RHEL-based (experiment-7, 2026-05-08)

### Symptom

`docker compose up --build` for experiment-7 failed during the Kafka image build:

```
 > [kafka 2/4] RUN apt-get update -qq && apt-get install -y --no-install-recommends openssl && rm -rf /var/lib/apt/lists/*:
0.312 /bin/sh: apt-get: command not found
------
failed to solve: process "/bin/sh -c apt-get update -qq && apt-get install -y ..." did not complete successfully: exit code: 127
```

All other images built and pushed successfully; only the Kafka TLS image failed.

### Root Cause

`kafka-tls.Dockerfile` was written assuming a Debian-based base image and used
`apt-get` to install `openssl`:

```dockerfile
FROM confluentinc/cp-kafka:7.6.1
USER root
RUN apt-get update -qq && apt-get install -y --no-install-recommends openssl && rm -rf /var/lib/apt/lists/*
```

`confluentinc/cp-kafka:7.6.1` is based on **Red Hat Enterprise Linux 8** (RHEL 8),
which uses `microdnf`/`dnf`/`yum` — not `apt-get`. Running `apt-get` produces
`command not found` with exit code 127.

The additional finding: `openssl` is **already present** at `/usr/bin/openssl` in
the cp-kafka image. The install step was unnecessary as well as wrong.

### Fix

Remove the `RUN apt-get ...` layer entirely from `kafka-tls.Dockerfile`:

```dockerfile
# BEFORE:
FROM confluentinc/cp-kafka:7.6.1
USER root
RUN apt-get update -qq && apt-get install -y --no-install-recommends openssl && rm -rf /var/lib/apt/lists/*
COPY experiments/experiment-7/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1001
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]

# AFTER:
FROM confluentinc/cp-kafka:7.6.1
USER root
COPY experiments/experiment-7/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1001
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]
```

### Guidance for Future Iterations

**Before adding a `RUN apt-get` (or any package manager command) to a Dockerfile,
verify the base image's OS distribution.**

Quick check:

```bash
docker run --rm <image> cat /etc/os-release
```

Confluent Platform images (`cp-kafka`, `cp-zookeeper`, `cp-schema-registry`, etc.)
are RHEL-based and ship `microdnf`. They do not have `apt-get`.

Specific checks:

1. **Check whether the tool is already present** before adding an install step:
   ```bash
   docker run --rm <image> which openssl
   ```
   If it exits 0, no installation is needed.

2. **If installation is genuinely required on a RHEL image**, use `microdnf`:
   ```dockerfile
   RUN microdnf install -y openssl && microdnf clean all
   ```

3. **Add this check to the Dockerfile comment** so future editors know the OS:
   ```dockerfile
   # confluentinc/cp-kafka:7.6.1 is RHEL 8-based; openssl ships at /usr/bin/openssl.
   ```

---

## EXP-014 — Missing `go.sum` files cause Docker build failures for new experiment modules (experiment-7, 2026-05-08)

### Symptom

`docker compose up --build` for experiment-7 failed during the Go build stage for
`data-provider-tls` and `robot-fleet-tls`:

```
 > [data-provider-tls builder 5/5] RUN go mod download && CGO_ENABLED=0 go build -o /app .:
1.597 main.go:37:2: missing go.sum entry for module providing package github.com/segmentio/kafka-go
1.597   (imported by arrowhead/experiment7/data-provider-tls); to add:
1.597     go get arrowhead/experiment7/data-provider-tls

 > [robot-fleet-tls builder 6/6] RUN go mod download && CGO_ENABLED=0 go build -o /app .:
1.515 /src/support/message-broker/broker.go:12:2: missing go.sum entry for module providing package github.com/rabbitmq/amqp091-go
1.515 main.go:43:2: missing go.sum entry for module providing package github.com/segmentio/kafka-go
```

The same problem was latent in four other experiment-7 modules (`cert-consumer`,
`cert-provisioner`, `cert-rest-authz`, `consumer-direct-tls`) that had not yet
reached the build stage.

### Root Cause

New Go modules were created with `go.mod` files listing external dependencies, but
`go mod tidy` was never run in those module directories. Without `go.sum`, Docker
multi-stage builds fail at `go mod download` because Go requires cryptographic
checksums for every dependency before it will fetch or use them.

`go build` and `go test` in the workspace succeeded locally because the Go workspace
(`go.work`) allows the workspace to satisfy indirect dependencies from other modules'
`go.sum` entries. The Docker build copies each module in isolation (no `go.work`) and
therefore has no fallback — the missing `go.sum` is fatal.

### Fix

Run `go mod tidy` in every module directory that is missing a `go.sum`:

```bash
cd experiments/experiment-7/services/data-provider-tls && go mod tidy
cd experiments/experiment-7/services/robot-fleet-tls   && go mod tidy
cd experiments/experiment-7/services/cert-consumer      && go mod tidy
cd experiments/experiment-7/services/cert-provisioner   && go mod tidy
cd experiments/experiment-7/services/cert-rest-authz    && go mod tidy
cd experiments/experiment-7/services/consumer-direct-tls && go mod tidy
```

Commit the generated `go.sum` files alongside the existing `go.mod` files.

### Guidance for Future Iterations

**After creating any new Go module, immediately run `go mod tidy` and commit the
resulting `go.sum` before building or shipping the module.**

The `go.work` workspace masks this problem locally: `go build ./...` from the repo
root succeeds even with missing `go.sum` files because workspace dependency
resolution is more permissive than module-isolated builds. Docker Dockerfiles build
each module in isolation without `go.work`, so they expose the gap immediately.

Specific checks:

1. **After creating a new module**, verify `go.sum` exists before committing:
   ```bash
   ls experiments/experiment-N/services/*/go.sum
   ```
   Any missing file needs `go mod tidy` run in that directory.

2. **Canary test**: build one Docker image for the new module immediately after
   creating it — do not wait until all services are ready. A failing `go mod download`
   at that stage confirms a missing `go.sum`.

3. **`go build ./...` from the workspace root is not sufficient** to detect missing
   `go.sum` files in individual modules. The only reliable check is either:
   - `go mod tidy` in each module directory, or
   - `docker compose build <service>` for at least one new service.

### Variant — Incomplete go.sum for modules that depend on workspace modules with external deps (experiment-13, 2026-05-14)

A subtler form of this failure occurred in experiment-13. The `pip` and `profile-ca`
services had `go.sum` files (created by the background agent), but Docker builds failed
with:

```
missing go.sum entry for module providing package google.golang.org/grpc
  (imported by arrowhead/experiment13/pip)
missing go.sum entry for module providing package google.golang.org/grpc/codes
  (imported by arrowhead/core-evol/proto/certlifecycle)
```

**Root cause**: The services imported `arrowhead/core-evol` (a workspace module via
`replace` directive), which itself imports `google.golang.org/grpc`. When the agent
ran tests locally, the workspace's shared dependency resolution populated the grpc
entries implicitly. But the service-level `go.sum` was written without the transitive
external dependencies of the replaced module.

In module-isolated Docker builds (no `go.work`), Go requires that *all* transitive
external dependencies appear in the root module's own `go.sum` — including those
pulled in by workspace-replaced modules. A `go.sum` that passes workspace-mode builds
may still be incomplete for isolated builds.

**Fix**: Running `go mod tidy` in the service directory adds the missing entries:
```bash
cd experiments/experiment-13/services/pip        && go mod tidy
cd experiments/experiment-13/services/profile-ca && go mod tidy
```

**Additional checklist item**: After creating any module that uses a `replace` directive
pointing to a workspace module, run `go mod tidy` and verify the resulting `go.sum`
includes the external dependencies of the replaced module — not just the module's own
direct imports.

---

## EXP-015 — Confluent Kafka `dub` requires `KAFKA_SSL_KEYSTORE_FILENAME`, not `KAFKA_SSL_KEYSTORE_LOCATION` (experiment-7, 2026-05-08)

### Symptom

`docker compose up` succeeded for all services except Kafka, which exited with code 1:

```
kafka-1  | [kafka-tls] SSL configured — starting Kafka
kafka-1  | ===> Configuring ...
kafka-1  | Running in KRaft mode...
kafka-1  | SSL is enabled.
kafka-1  | KAFKA_SSL_KEYSTORE_FILENAME is required.
kafka-1  | Command [/usr/local/bin/dub ensure KAFKA_SSL_KEYSTORE_FILENAME] FAILED !
```

The entrypoint script completed successfully (keystores were created, SSL env vars were
exported), but Kafka itself refused to start.

### Root Cause

The Confluent Platform Docker image uses a configuration tool called `dub` (Docker
Utility Bridge) to translate `KAFKA_*` environment variables into `server.properties`
entries. `dub` enforces its own set of required variable names.

For SSL, `dub` expects:

- `KAFKA_SSL_KEYSTORE_FILENAME` — a **bare filename** resolved relative to
  `/etc/kafka/secrets/`
- `KAFKA_SSL_TRUSTSTORE_FILENAME` — same pattern

The entrypoint script was instead exporting:

- `KAFKA_SSL_KEYSTORE_LOCATION="/tmp/kafka-keystore.p12"` — an absolute path

`dub` does not recognise `*_LOCATION` for file-based SSL config; it only accepts
`*_FILENAME`. Because `KAFKA_SSL_KEYSTORE_FILENAME` was absent, `dub`'s
`ensure` check failed and Kafka exited before writing `server.properties`.

### Fix

1. Write keystores to `/etc/kafka/secrets/` (the directory `dub` resolves filenames
   against) instead of `/tmp/`.
2. Export `KAFKA_SSL_KEYSTORE_FILENAME` and `KAFKA_SSL_TRUSTSTORE_FILENAME` as bare
   filenames, not absolute paths.

```bash
# BEFORE:
KEYSTORE_PATH="/tmp/kafka-keystore.p12"
TRUSTSTORE_PATH="/tmp/kafka-truststore.p12"
...
export KAFKA_SSL_KEYSTORE_LOCATION="$KEYSTORE_PATH"
export KAFKA_SSL_TRUSTSTORE_LOCATION="$TRUSTSTORE_PATH"

# AFTER:
SECRETS_DIR="/etc/kafka/secrets"
KEYSTORE_PATH="$SECRETS_DIR/kafka-keystore.p12"
TRUSTSTORE_PATH="$SECRETS_DIR/kafka-truststore.p12"
mkdir -p "$SECRETS_DIR"
...
export KAFKA_SSL_KEYSTORE_FILENAME="kafka-keystore.p12"
export KAFKA_SSL_TRUSTSTORE_FILENAME="kafka-truststore.p12"
```

Applied to `experiments/experiment-7/dockerfiles/kafka-tls-entrypoint.sh`.

### Guidance for Future Iterations

**When configuring SSL on `confluentinc/cp-kafka`, use `KAFKA_SSL_KEYSTORE_FILENAME`
and place the keystore file in `/etc/kafka/secrets/` — never use
`KAFKA_SSL_KEYSTORE_LOCATION` with an absolute path.**

The `dub` variable name convention:

| What you want | Wrong variable | Correct variable |
|---|---|---|
| Keystore file | `KAFKA_SSL_KEYSTORE_LOCATION` | `KAFKA_SSL_KEYSTORE_FILENAME` |
| Truststore file | `KAFKA_SSL_TRUSTSTORE_LOCATION` | `KAFKA_SSL_TRUSTSTORE_FILENAME` |

Both `*_FILENAME` variables are bare names (e.g. `kafka-keystore.p12`); `dub`
prepends `/etc/kafka/secrets/` automatically when generating `server.properties`.

Specific checks:

1. **Search Confluent documentation and GitHub for `dub ensure`** before setting
   any `KAFKA_SSL_*` environment variable — the required variable names are enforced
   at startup and failures are silent until `dub` runs.

2. **Check `/etc/kafka/secrets/` exists** in the entrypoint before writing keystores:
   ```bash
   mkdir -p /etc/kafka/secrets
   ```

3. **`KAFKA_SSL_KEYSTORE_LOCATION` is a valid Kafka broker config key** (in
   `server.properties`) but it is NOT a valid `dub` environment variable. The two
   naming systems are different and the error message only reveals the `dub` side.

---

## EXP-016 — Confluent Kafka SSL truststore never loaded; four interacting bugs (experiment-7, 2026-05-08)

### Symptom

Kafka kept failing at startup with `PKIX path building failed: unable to find valid
certification path to requested target` even after the keystore and truststore were
visually confirmed correct via `keytool -list` and `openssl verify`. Every fix attempt
revealed a new layer of the same root failure.

### Root Causes (layered)

Four separate bugs combined to produce the same `PKIX path building failed` error:

**Bug 1 — `KAFKA_INTER_BROKER_LISTENER_NAME` absent.**
Kafka defaults `inter.broker.listener.name` to `PLAINTEXT`, which is not in
`advertised.listeners` (only `SSL` is). This caused an `IllegalArgumentException`
at startup before SSL validation even ran.
Fix: add `KAFKA_INTER_BROKER_LISTENER_NAME: SSL` to docker-compose.yml.

**Bug 2 — `dub` does NOT translate `KAFKA_SSL_TRUSTSTORE_FILENAME` to `ssl.truststore.location`.**
`dub` (Confluent's Docker config tool) translates `KAFKA_SSL_KEYSTORE_FILENAME` →
`ssl.keystore.location=/etc/kafka/secrets/<name>`, but performs no equivalent
translation for `KAFKA_SSL_TRUSTSTORE_FILENAME`. The generated `kafka.properties`
contained `ssl.truststore.filename=...` (unused by Kafka) but no
`ssl.truststore.location`. Kafka therefore used the JVM default `cacerts` trust store,
which does not contain our custom CA, causing the `PKIX path building failed` error.
Fix: set `KAFKA_SSL_TRUSTSTORE_LOCATION` (absolute path) in the entrypoint script instead.

**Bug 3 — `openssl pkcs12 -nokeys` does not produce Java-readable trusted entries.**
The original truststore was created with `openssl pkcs12 -export -nokeys -in ca.crt`.
OpenSSL does not set the `trustedKeyUsage` attribute on the CA entry. Java's SSL engine
requires this attribute to recognise a certificate as a trust anchor. Even if the
truststore path had been correctly passed to Kafka, it would have been empty from Java's
perspective.
Fix: use `keytool -importcert -trustcacerts` to create the truststore, which correctly
marks the CA cert as a `trustedCertEntry`.

**Bug 4 — Stale keystore/truststore files persist on container restart.**
The entrypoint script wrote keystores to `/etc/kafka/secrets/` (inside the container
filesystem). When Docker restarts a container without recreating it, the old files remain.
On the next run, `set -e` caused the script to exit when keytool reported "alias already
exists" instead of overwriting the stale truststore.
Fix: add `rm -f "$KEYSTORE_PATH" "$TRUSTSTORE_PATH"` at the top of the keystore creation
section.

**Bonus — Hostname verification.**
Kafka 2.0+ defaults `ssl.endpoint.identification.algorithm` to `HTTPS`, which requires
the server cert to have a SAN matching the hostname. Our CA does not issue SANs. This
would have caused a separate failure after the above bugs were fixed.
Fix: set `KAFKA_SSL_ENDPOINT_IDENTIFICATION_ALGORITHM: ""` in docker-compose.yml.

### Final working SSL configuration pattern

**Entrypoint script:**

```bash
# Remove stale files from previous container run
rm -f "$KEYSTORE_PATH" "$TRUSTSTORE_PATH"

# PKCS12 keystore — openssl is fine here (Java reads the private key entry correctly)
openssl pkcs12 -export \
  -in "$CERT_DIR/kafka.crt" -inkey "$CERT_DIR/kafka.key" \
  -out "$KEYSTORE_PATH" -passout "pass:$PASS" -name kafka

# JKS truststore — MUST use keytool (sets trusted-key-usage attribute)
keytool -importcert -alias ca -file "$CERT_DIR/ca.crt" \
  -keystore "$TRUSTSTORE_PATH" -storetype JKS -storepass "$PASS" -noprompt

# Keystore: use FILENAME (dub translates to ssl.keystore.location)
export KAFKA_SSL_KEYSTORE_TYPE=PKCS12
export KAFKA_SSL_KEYSTORE_FILENAME="kafka-keystore.p12"
export KAFKA_SSL_KEYSTORE_CREDENTIALS="kafka_keystore_creds"
export KAFKA_SSL_KEY_CREDENTIALS="kafka_key_creds"

# Truststore: use LOCATION (dub does NOT translate TRUSTSTORE_FILENAME)
export KAFKA_SSL_TRUSTSTORE_TYPE=JKS
export KAFKA_SSL_TRUSTSTORE_LOCATION="$TRUSTSTORE_PATH"
export KAFKA_SSL_TRUSTSTORE_PASSWORD="$PASS"
```

**docker-compose.yml additions:**

```yaml
KAFKA_INTER_BROKER_LISTENER_NAME: SSL
KAFKA_SSL_ENDPOINT_IDENTIFICATION_ALGORITHM: ""   # our CA issues no SANs
```

### Guidance for Future Iterations

1. **`dub` variable asymmetry**: `KAFKA_SSL_KEYSTORE_FILENAME` is translated to
   `ssl.keystore.location`; `KAFKA_SSL_TRUSTSTORE_FILENAME` is NOT. Always verify
   the generated `kafka.properties` with a test run before debugging SSL errors:
   ```bash
   docker run --rm -e KAFKA_SSL_TRUSTSTORE_FILENAME=foo ... \
     confluentinc/cp-kafka:7.6.1 /etc/confluent/docker/configure \
     && grep ssl /etc/kafka/kafka.properties
   ```

2. **Always use `keytool` for Java truststores.** `openssl pkcs12 -nokeys` produces
   cert entries that Java's `X509TrustManager` silently ignores.

3. **`PKIX path building failed` on Kafka startup almost always means the truststore
   is missing or unreadable** — not that the cert chain is actually broken. Verify
   with `openssl verify -CAfile ca.crt cert.crt` first; if that passes, the problem
   is truststore configuration, not the certs.

4. **Stale container filesystem:** add `rm -f` before recreating keystores in
   entrypoint scripts. Docker restarts preserve the container filesystem; `docker
   compose up -d` does not recreate containers unless the image or config changed.

5. **Set `ssl.endpoint.identification.algorithm=` (empty)** when the CA does not issue
   SANs. The default `HTTPS` value causes Kafka to reject its own cert at startup.

---

## EXP-017 — policy-sync port not exposed and wrong env var name caused test pre-flight failure (experiment-7, 2026-05-08)

### Symptom

`test-system.sh` pre-flight failed on `policy-sync synced=true` after waiting 30 s, even
though `docker compose logs policy-sync` clearly showed repeated `sync OK` lines. The
test reported "not synced after 30s — check policy-sync container logs."

```
  Waiting for policy-sync first sync (up to 30s)...
  ... attempt 1/6, sleeping 5s
  ...
  ... attempt 6/6, sleeping 5s
  FAIL  policy-sync synced=true
         not synced after 30s — check policy-sync container logs
```

### Root Cause

Two independent bugs in `docker-compose.yml` for the policy-sync service:

**Bug 1 — Port 9095 not published to the host.**
The policy-sync service had no `ports:` section. The test script calls
`curl http://localhost:9095/status` from the host, which connects to the host's
loopback — not the Docker network. Without a port mapping, this connection is always
refused, so `ps_status` was always `{}` and the `"synced":true` check never passed.

**Bug 2 — Wrong environment variable name: `CA_URL` instead of `CONSUMERAUTH_URL`.**
The policy-sync support module reads `CONSUMERAUTH_URL` (the ConsumerAuthorization base
URL from which it fetches grants). The docker-compose.yml was setting `CA_URL` instead.
Because the wrong variable was silently ignored and the default value
(`http://consumerauth:8082`) happens to be identical to what was configured, policy-sync
still connected correctly — making this bug invisible at runtime but incorrect and
misleading in configuration.

### Fix

In `docker-compose.yml`, for the `policy-sync` service:
1. Add `ports: - "9095:9095"`
2. Rename `CA_URL` → `CONSUMERAUTH_URL`

```yaml
# BEFORE:
policy-sync:
  environment:
    CA_URL: "http://consumerauth:8082"
    ...
  # (no ports: section)

# AFTER:
policy-sync:
  environment:
    CONSUMERAUTH_URL: "http://consumerauth:8082"
    ...
  ports:
    - "9095:9095"
```

### Guidance for Future Iterations

**Verify the `ports:` mapping in docker-compose.yml for every service whose endpoints
are called by `test-system.sh`.**

The `test-system.sh` script runs on the host and reaches services via `localhost:<port>`.
A service that only exposes an internal Docker network port is unreachable from the test
script even if every container inside the stack can reach it.

Specific checks:

1. **For every `curl`/`http_code` call in `test-system.sh`**, confirm the target port
   is in `ports:` for the relevant service in docker-compose.yml:
   ```bash
   grep "localhost:" test-system.sh | grep -oP ':\d+' | sort -u
   # Then verify each port appears in docker-compose.yml under ports:
   ```

2. **Environment variable names must match what the service binary reads**, not what
   sounds intuitive. Always cross-check against the service's `main.go`:
   ```bash
   grep 'envOr\|os.Getenv' experiments/experiment-N/services/<name>/main.go
   ```
   A wrong env var name has two failure modes:
   - **Silent** — the Go code has a matching default, the service starts but uses the
     wrong value (e.g. `CA_URL` instead of `CONSUMERAUTH_URL` where the default happens
     to be the same address).
   - **Fatal** — the variable is required (`os.Getenv` with no default and an explicit
     `log.Fatal`), the service exits immediately with "X is required" (e.g. `TARGET_URL`
     set in docker-compose instead of the required `UPSTREAM_URL` in pki-rest-authz,
     experiment-13). Both cases have identical root cause; only the visibility differs.

3. **Distinguish "service is syncing" from "test can reach /status"** — healthy
   container logs and a test pre-flight failure can coexist when the port is missing.
   Always check `curl localhost:<port>/health` from the HOST before running the test
   suite.

---

## EXP-018 — cert-rest-authz proxied upstream requests using `http.DefaultClient`, bypassing custom CA pool (experiment-7, 2026-05-08)

### Symptom

cert-rest-authz logs show successful XACML decisions (`PERMIT consumer="cert-consumer"`) but
every forwarded request to data-provider-tls fails immediately:

```
upstream error: Get "https://data-provider-tls:9094/telemetry/latest":
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

cert-consumer polls and receives 502 from cert-rest-authz. `msgCount` stays at 0. The
AuthzForce path is completely correct; only the upstream proxy step fails.

### Root Cause

`server.go:proxyRequest()` called `http.DefaultClient.Do(req)`. `http.DefaultClient` uses
the system root CAs, which do not include the ephemeral Arrowhead CA certificate (generated
fresh at every `ca` container start).

Meanwhile, `tlsconfig.go` already contained `buildClientTLSConfig(cert, caPool)` and
`buildMTLSUpstreamClient(tlsCfg)` — the right helpers existed but were never wired up.
`main.go` built the CA pool and own cert for the **inbound** server TLS config but did not
pass them to the upstream HTTP client used for **outbound** requests.

### Fix

1. Add `upstreamClient *http.Client` field to `certAuthzServer`.
2. Pass it as a parameter to `newCertAuthzServer(...)`.
3. In `main.go`, build the upstream client via the existing helpers:
   ```go
   upstreamTLSCfg := buildClientTLSConfig(ownCert, caPool)
   upstreamClient := buildMTLSUpstreamClient(upstreamTLSCfg)
   srv := newCertAuthzServer(cfg, azClient, cache, upstreamClient)
   ```
4. In `proxyRequest`, replace `http.DefaultClient.Do(req)` with `s.upstreamClient.Do(req)`.

The CA pool (fetched at startup via `GET /ca/info`) is populated with the live CA root
certificate, so the outbound TLS verifier now trusts certs issued by the same CA instance.

### Guidance for Future Iterations

**Every TLS-proxying service must use a custom HTTP client for outbound requests.**

`http.DefaultClient` uses the system root CA store. An ephemeral self-signed CA (like the
Arrowhead CA) is never in that store. A service that:
- Fetches the CA cert at startup (for its own TLS setup)
- Then proxies requests to other services over HTTPS

…must configure a custom `*http.Client` with `RootCAs` set to the fetched CA pool, and use
that client for ALL outbound HTTPS requests. Using `http.DefaultClient` silently works in
tests (if tests use plaintext or `InsecureSkipVerify`) but fails against real mTLS upstreams.

Pattern to apply consistently:
```go
// At startup: fetch CA cert, issue own cert
caPool, _ := fetchCACertWithRetry(caURL, 10)
ownCert    := issueCertWithRetry(caURL, systemName, 10)

// For inbound mTLS (server verifies client certs)
serverTLS := buildServerTLSConfig(ownCert, caPool)

// For outbound HTTPS (client verifies server cert)
upstreamTLS    := buildClientTLSConfig(ownCert, caPool)
upstreamClient := buildMTLSUpstreamClient(upstreamTLS)
// … pass upstreamClient to the handler; never use http.DefaultClient for upstream calls
```

---

## EXP-019 — mTLS test used `localhost` hostname, failing cert hostname verification; probe key extraction used mismatched line numbers (experiment-7, 2026-05-08)

### Symptom

Section 7 of `test-system.sh` (mTLS direct curl test) always returned HTTP `000000` for
both the authorized and unauthorized cases:

```
waiting for data-provider-tls data... (attempt 1/12, HTTP 000000)
...
FAIL  mTLS test-probe GET /telemetry/latest → 200 (authorized)
FAIL  mTLS: request without client cert rejected
```

### Root Cause

**Bug A — hostname mismatch:** cert-rest-authz is issued a certificate with
`CN=cert-rest-authz` and `SAN=cert-rest-authz`. The test script called:

```bash
curl ... https://localhost:9098/telemetry/latest
```

curl connects to `localhost` and verifies the server cert against that name. The cert only
has `cert-rest-authz` as a SAN, so curl rejects the connection:

```
* subjectAltName does not match localhost
* SSL: no alternative certificate subject name matches target host name 'localhost'
```

Exit code 60 from curl; with `-w "%{http_code}"` curl still outputs `000`, then
`|| echo "000"` appends another `000`, giving `000000`.

**Bug B — doubled `000` for the no-cert case:** `curl -s -o /dev/null -w "%{http_code}"`
always outputs the HTTP status code string, including `000` when the connection fails.
The pattern `|| echo "000"` then appended a second `000`, so `$no_cert_code` was `000000`
instead of `000`, and the `[ "$no_cert_code" = "000" ]` check never matched.

**Bug C — broken key extraction:** The original key extraction used:

```bash
probe_key=$(echo "$probe_resp" | sed -n '/^-----BEGIN/,/^-----END/p' | \
  tail -n +$(echo "$probe_resp" | grep -n "BEGIN" | sed -n '2p' | cut -d: -f1))
```

`grep -n "BEGIN"` counts lines in the full response (including empty lines and the `---`
separator). `tail -n +N` was applied to the `sed` output (which only contains PEM blocks,
with no empty lines or separator). The line offsets didn't match, so the key was truncated
to its last 2–3 lines, missing the `-----BEGIN EC PRIVATE KEY-----` header. Curl then
failed with `unable to set private key file`.

### Fix

**Bug A:** Use `--resolve "cert-rest-authz:9098:127.0.0.1"` so curl maps the hostname to
localhost's IP without DNS, while sending the correct SNI and verifying against
`cert-rest-authz`:
```bash
curl ... --resolve "cert-rest-authz:9098:127.0.0.1" https://cert-rest-authz:9098/...
```

**Bug B:** Replace `|| echo "000"` with `; echo -n ""` (a no-op that resets the exit
code) for TLS-connection tests. The curl already outputs `000` on connection failure;
suppressing the `|| echo` prevents doubling.

**Bug C:** Use python3 to extract the cert and key fields separately:
```bash
probe_resp_json=$(curl -s ...)
probe_cert=$(echo "$probe_resp_json" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["certificate"])')
probe_key=$(echo "$probe_resp_json"  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["privateKey"])')
```

### Guidance for Future Iterations

1. **Never use `https://localhost:<port>` in test scripts for services with service-name SANs.**
   Use `--resolve <hostname>:<port>:127.0.0.1` to map the Docker service hostname to the
   loopback address. The TLS handshake then succeeds because the hostname matches the SAN.

2. **`curl -w "%{http_code}"` already outputs `000` on connection failure.** Do not combine
   with `|| echo "000"` — this doubles the string to `000000`, breaking equality checks.
   Use `; true` or `; echo -n ""` if you need to suppress the non-zero exit code.

3. **Extract JSON fields with `python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["field"])'`
   rather than `sed`/`grep` + line-number arithmetic.** Shell line-counting across filtered
   and unfiltered versions of the same stream is fragile; Python JSON parsing is exact.

---

## EXP-020 — Docker `ARG` declared before `FROM` is out of scope inside the build stage (experiment-9, 2026-05-14)

### Symptom

`docker compose up --build` failed for all four Arrowhead core system services
(`serviceregistry`, `authentication`, `consumerauth`, `dynamicorch`) with the same error:

```
 > [consumerauth builder 4/4] RUN CGO_ENABLED=0 go build -o /app ./cmd/${CMD}:
0.345 no Go files in /src/cmd
------
failed to solve: process "/bin/sh -c CGO_ENABLED=0 go build -o /app ./cmd/${CMD}" did not complete successfully: exit code: 1
```

The `${CMD}` variable expanded to an empty string, so Go tried to build `./cmd/` (the
directory itself) — which contains no `.go` files.

### Root Cause

`core.Dockerfile` declared `ARG CMD` **before** the `FROM` instruction:

```dockerfile
ARG CMD                          # ← declared in global scope
FROM golang:1.22-alpine AS builder
WORKDIR /src
...
RUN CGO_ENABLED=0 go build -o /app ./cmd/${CMD}   # ← CMD is empty here
```

In Docker's multi-stage build model, `ARG` instructions before `FROM` define
**global build args** that are only in scope for `FROM` instructions (e.g., to
parameterise the base image tag). They are **not automatically available** inside
any subsequent build stage. After a `FROM` line, the arg is out of scope and expands
to an empty string unless it is re-declared inside that stage.

The experiment-8 `core.Dockerfile` had the correct order — `ARG CMD` after `FROM` —
and was copied incorrectly when creating the experiment-9 equivalent.

### Fix

Move `ARG CMD` to after `FROM golang:1.22-alpine AS builder`:

```dockerfile
# BEFORE (broken):
ARG CMD
FROM golang:1.22-alpine AS builder
WORKDIR /src
...

# AFTER (correct):
FROM golang:1.22-alpine AS builder
ARG CMD
WORKDIR /src
...
```

Applied to `experiments/experiment-9/dockerfiles/core.Dockerfile`.

### Guidance for Future Iterations

**`ARG` declared before `FROM` is only visible in `FROM` statements — for example,
to parameterise the base image tag. Any `ARG` used inside a build stage (in `RUN`,
`COPY`, `ENV`, etc.) must be declared after the `FROM` that opens that stage.**

Quick reference:

```dockerfile
# Valid use of global ARG (base image parameterisation):
ARG GO_VERSION=1.22
FROM golang:${GO_VERSION}-alpine AS builder

# Correct use for build-time variables inside a stage:
FROM golang:1.22-alpine AS builder
ARG CMD                        # ← re-declare (or declare for first time) after FROM
RUN go build ./cmd/${CMD}      # ← CMD is now in scope
```

Specific checks:

1. **Search all Dockerfiles for `ARG` lines that appear before any `FROM`:**
   ```bash
   grep -n "^ARG\|^FROM" experiments/experiment-9/dockerfiles/*.Dockerfile \
     | awk -F: '{print $1, $2, $3}' | sort
   ```
   Any `ARG` that appears at a lower line number than the first `FROM` and is also
   used in a `RUN`/`COPY`/`ENV` line is almost certainly a scoping bug.

2. **When copying a Dockerfile from a previous experiment**, diff it against the
   source to confirm `ARG` / `FROM` ordering was not accidentally reversed.

3. **The symptom is always a variable expanding to empty string** — `${CMD}` becomes
   `""`, so `./cmd/` is built instead of `./cmd/serviceregistry`. The error message
   `no Go files in /src/cmd` is the tell.

---

## EXP-021 — nginx exits at startup with "host not found in upstream" when backend containers are not yet running (experiment-9, 2026-05-14)

### Symptom

`http://localhost:3009/` (the experiment-9 dashboard) returns **connection refused**.
The dashboard container exits immediately after starting with exit code 1:

```
[emerg] 1#1: host not found in upstream "serviceregistry" in /etc/nginx/conf.d/default.conf:9
nginx: [emerg] host not found in upstream "serviceregistry" ...
```

This happens even though the dashboard is a static HTML page that has no functional
dependency on the ServiceRegistry.

### Root Cause

When nginx parses a `proxy_pass http://hostname/;` directive with a **literal hostname**
(not a variable), it resolves that hostname via DNS **at startup**, before it serves any
requests. If the hostname does not resolve — because the backend container has not yet
been registered in Docker's DNS — nginx aborts with `host not found in upstream` and exits.

Docker's embedded DNS only registers a container name once Docker has started (or at least
scheduled) that container. If the dashboard starts before any of the backend services, all
their hostnames are unknown to Docker DNS and every `proxy_pass` target fails to resolve.

This is compounded by the earlier `depends_on` fix (see EXP-021b): once the dashboard no
longer waits for slow-starting services, it can start early — before backends are up —
which is exactly when the startup-resolution problem triggers.

### Fix

Three rules must all hold together:

**1. Add `resolver 127.0.0.11 valid=5s ipv6=off;` at server-block level.**
`127.0.0.11` is Docker's embedded DNS, always present inside containers on a user-defined
network. `valid=5s` re-checks DNS frequently so newly started backends become reachable.

**2. Use a `set $upstream` variable in every `proxy_pass`** so nginx defers DNS resolution
to request time instead of startup time.

**3. Use `rewrite ... break` to strip the location prefix**, and put `set $upstream`
**before** the `rewrite` — `rewrite ... break` stops processing subsequent `set`
directives in the same rewrite phase, so a `set` after `rewrite` is silently skipped,
leaving `$upstream` uninitialized and causing a 500 error.

**Do NOT use `$request_uri` in `proxy_pass`** — `$request_uri` is the full original URI
including the location prefix (e.g. `/api/serviceregistry/health`), so the backend
receives the wrong path and returns 404.

```nginx
# BEFORE (resolves at startup — nginx crashes if backend is not up):
location /api/serviceregistry/ {
    proxy_pass http://serviceregistry:8080/;
}

# WRONG variable attempt 1 — $request_uri includes /api/serviceregistry/ prefix → 404:
location /api/serviceregistry/ {
    set $upstream http://serviceregistry:8080;
    proxy_pass $upstream$request_uri;
}

# WRONG variable attempt 2 — set after rewrite break → $upstream uninitialized → 500:
location /api/serviceregistry/ {
    rewrite ^/api/serviceregistry/(.*) /$1 break;
    set $upstream http://serviceregistry:8080;   # never executed!
    proxy_pass $upstream;
}

# CORRECT — set before rewrite, rewrite strips prefix, proxy_pass uses rewritten URI:
resolver 127.0.0.11 valid=5s ipv6=off;

location /api/serviceregistry/ {
    set $upstream http://serviceregistry:8080;      # 1. assign variable first
    rewrite ^/api/serviceregistry/(.*) /$1 break;  # 2. strip prefix, stop rewrites
    proxy_pass         $upstream;                  # 3. forward rewritten URI to backend
    proxy_set_header   Host serviceregistry:8080;
    proxy_read_timeout 5s;
}
```

Applied to `experiments/experiment-9/dashboard/nginx.conf`.

### Guidance for Future Iterations

**Any nginx dashboard that uses `proxy_pass` with Docker service hostnames must use the
variable pattern — never a literal hostname — so nginx starts independently of whether
the backends are up.**

Template for every proxied location:

```nginx
resolver 127.0.0.11 valid=5s ipv6=off;  # declare once at server block level

location /api/myservice/ {
    set $target http://myservice:8080;
    proxy_pass         $target$request_uri;
    proxy_set_header   Host myservice:8080;
    proxy_read_timeout 5s;
}
```

Specific checks:

1. **Test the dashboard container in isolation** (no compose network, no backend services):
   ```bash
   docker build -t test-dashboard .
   docker run --rm -p 3099:80 test-dashboard
   curl http://localhost:3099/   # must return 200, not connection refused
   ```
   If nginx exits, run `docker logs <id>` — "host not found in upstream" is this bug.

2. **Never use a literal hostname in `proxy_pass`** in a dashboard nginx.conf. The rule
   applies even if `depends_on` ensures backends are started first — container startup order
   and DNS registration are not perfectly synchronised.

3. **`resolver 127.0.0.11`** is the Docker embedded DNS address. It is always present
   inside any container on a user-defined Docker network. It is NOT available outside Docker
   (e.g. in `nginx -t` on the host). Do not use `resolver 8.8.8.8` — that resolves public
   DNS, not Docker service names.

4. **`$request_uri` preserves the full original path and query string** after stripping the
   location prefix via `proxy_pass`. Use it instead of bare `$target/` to avoid
   double-path or dropped query-string bugs.

---

## EXP-021b — Dashboard unreachable because `depends_on` propagates slow service chains (experiment-9, 2026-05-14)

### Symptom

`http://localhost:3009/` (the experiment-9 dashboard) returns **connection refused** even
after all core services are reported healthy and `docker compose ps` shows them running.
The dashboard container has not started at all — it is stuck waiting on dependencies.

### Root Cause

The dashboard's `depends_on` block listed `pki-rest-authz: condition: service_started`
and `portal-cloud-ml: condition: service_started`.

`condition: service_started` sounds lightweight, but Docker only considers a container
"started" once Docker has actually run it. Docker will not run `pki-rest-authz` until its
own `depends_on` conditions are met:

```
dashboard
  └── pki-rest-authz (service_started)
        └── portal-cloud-ml (service_healthy)   ← healthcheck retries: 15, start_period: 15s
              └── kafka-authz (service_healthy)
                    └── kafka (service_healthy)  ← healthcheck retries: 20, start_period: 30s
```

Until the entire chain resolves, the dashboard container is never started. If any service
in the chain fails its healthcheck permanently (e.g. a PKI lifecycle error in
`portal-cloud-ml`), the dashboard never becomes reachable — making it impossible to even
open the dashboard to diagnose the problem.

The same logic applied to `kafka-authz: service_started` and `policy-sync: service_started`.

### Fix

Remove all application-service entries from the dashboard's `depends_on`. The dashboard
is a **static HTML + nginx** container — it does not need any backend service to be running
in order to serve the HTML. Nginx returns `502 Bad Gateway` for proxy locations whose
backends are not yet up; the dashboard JavaScript interprets those as "unhealthy" and shows
the appropriate status. Only `cert-provisioner: service_completed_successfully` is kept,
because the `cert-provisioner` one-shot container must have completed before the certs
volume is in a consistent state (even though the dashboard nginx doesn't use the certs).

```yaml
# BEFORE (blocks dashboard until deep service chain resolves):
dashboard:
  depends_on:
    cert-provisioner:
      condition: service_completed_successfully
    consumerauth:
      condition: service_started
    authzforce:
      condition: service_started
    policy-sync:
      condition: service_started
    kafka-authz:
      condition: service_started
    pki-rest-authz:
      condition: service_started
    portal-cloud-ml:
      condition: service_started

# AFTER (starts as soon as cert-provisioner finishes — seconds, not minutes):
dashboard:
  depends_on:
    cert-provisioner:
      condition: service_completed_successfully
```

### Guidance for Future Iterations

**A static file / nginx dashboard must have minimal `depends_on` — it should start
immediately and let the UI reflect service health, not be blocked waiting for service health.**

The general rule:

| Service type | Correct `depends_on` |
|---|---|
| Static HTML/nginx dashboard | Only init containers (`cert-provisioner`, etc.) |
| Application service (needs auth) | Its direct runtime dependencies only |
| Test/setup containers | All services they call |

Specific checks:

1. **Never list a service with a deep `depends_on` chain in a dashboard's `depends_on`.**
   `condition: service_started` is NOT the same as "starts quickly". Docker won't start
   container B until A is started, and won't start A until A's own conditions are met.
   Trace the full transitive chain before adding any entry.

2. **A dashboard that can't start makes it impossible to diagnose why other services
   are down.** Monitoring UIs must be available even when the things they monitor are not.

3. **Verify dashboard availability early in `test-system.sh`** (pre-flight section):
   ```bash
   smoke_http "dashboard reachable" "http://localhost:3009/"
   ```
   If this fails, all later assertions about service health are moot.

4. **`docker compose ps`** will show the dashboard container as `Created` (not `Running`)
   if its `depends_on` conditions are unsatisfied — the symptom looks identical to a
   crashed container, so check the dependency chain first before investigating nginx or
   the Dockerfile.

---

## EXP-024 — `/dev/tcp` healthcheck fails in Alpine containers (`/bin/sh` has no `/dev/tcp`) (experiment-13, 2026-05-14)

### Symptom

A gRPC service (or any TCP-only service) running in an Alpine container is marked
**unhealthy** by Docker even though the service started and is accepting connections:

```
container experiment-13-authz-pdp-1 is unhealthy
```

The container logs show the service running correctly:

```
authz-pdp: gRPC server listening on :9550 (reflection enabled)
```

Running `docker inspect` reveals the health check is failing with:

```
/bin/sh: can't create /dev/tcp/localhost/9550: nonexistent directory
```

### Root Cause

The healthcheck command used the bash TCP pseudo-device:

```yaml
test: ["CMD-SHELL", "printf '' > /dev/tcp/localhost/9550 2>/dev/null && echo ok || exit 1"]
```

`/dev/tcp` is a bash-specific feature — it is not a real filesystem path but a
special construct that bash intercepts. Alpine Linux containers use busybox `ash`
as `/bin/sh`, which does not support `/dev/tcp`. The shell literally tries to open
`/dev/tcp/localhost/9550` as a file path, fails because the directory doesn't exist,
and exits non-zero.

The same healthcheck works in `confluentinc/cp-kafka` containers because those
images use a full bash shell.

### Fix

Replace the `/dev/tcp` check with `nc -z` (netcat), which is included in Alpine's
busybox and works under `sh`:

```yaml
# BEFORE (bash-only):
test: ["CMD-SHELL", "printf '' > /dev/tcp/localhost/9550 2>/dev/null && echo ok || exit 1"]

# AFTER (works in Alpine sh / busybox):
test: ["CMD-SHELL", "nc -z localhost 9550 || exit 1"]
```

No Dockerfile change needed — `nc` is part of Alpine's base busybox install.

### Guidance for Future Iterations

`/dev/tcp` healthchecks only work in containers whose base image provides bash
(e.g. Debian, Ubuntu, or Confluent's cp-kafka). Never use `/dev/tcp` in healthchecks
for Alpine-based or scratch-based containers.

| Base image | Shell | `/dev/tcp` works? | TCP health check to use |
|---|---|---|---|
| `confluentinc/cp-kafka` | bash | Yes | `/dev/tcp` or `nc -z` |
| `alpine:*` | busybox sh | No | `nc -z localhost <port>` |
| `golang:*-alpine` | busybox sh | No | `nc -z localhost <port>` |
| `scratch` / distroless | none | No | `COPY` a probe binary |

For HTTP services on Alpine, `wget -qO- http://localhost:<port>/health` is the
correct approach (wget is typically installed explicitly in the Dockerfile).
For TCP-only services (gRPC, raw TCP), use `nc -z localhost <port>`.

---

## EXP-023 — `dub` requires `KAFKA_SSL_TRUSTSTORE_FILENAME` when `ssl.client.auth=required` (experiment-13, 2026-05-14)

### Symptom

Kafka container exits with code 1 immediately after the entrypoint script completes
cert setup successfully:

```
[kafka-tls] SSL configured — starting Kafka
===> Configuring ...
Running in KRaft mode...
SSL is enabled.
KAFKA_SSL_TRUSTSTORE_FILENAME is required.
Command [/usr/local/bin/dub ensure KAFKA_SSL_TRUSTSTORE_FILENAME] FAILED !
```

All other containers that depend on Kafka are created but never start.

### Root Cause

The experiment-12 `kafka-tls-entrypoint.sh` explicitly unsets `KAFKA_SSL_TRUSTSTORE_FILENAME`
and uses the absolute-path alternative `KAFKA_SSL_TRUSTSTORE_LOCATION` instead, based on the
finding in EXP-015/EXP-016 that dub does not translate the truststore filename.

This worked for experiments 7, 9, and 12 because all three set `KAFKA_SSL_CLIENT_AUTH: none`.
When client auth is `none`, dub does not validate that a truststore is configured and the missing
`KAFKA_SSL_TRUSTSTORE_FILENAME` variable is never checked.

Experiment-13 sets `KAFKA_SSL_CLIENT_AUTH: required` so that Kafka enforces mTLS on all
connections and the client certificate CN becomes the XACML subject-id. With client auth
enabled, dub runs `dub ensure KAFKA_SSL_TRUSTSTORE_FILENAME` to confirm that the truststore
is configured — and fails because the entrypoint unset it.

### Fix

Create an experiment-13-specific entrypoint that:
1. Exports `KAFKA_SSL_TRUSTSTORE_FILENAME="kafka-truststore.jks"` (bare filename, resolved
   relative to `/etc/kafka/secrets/` by dub)
2. Exports `KAFKA_SSL_TRUSTSTORE_CREDENTIALS` pointing to the truststore password file
3. Removes `unset KAFKA_SSL_TRUSTSTORE_FILENAME` and `KAFKA_SSL_TRUSTSTORE_LOCATION`

```bash
# experiment-13 entrypoint (after truststore is created in /etc/kafka/secrets/)
printf '%s' "$KAFKA_KEYSTORE_PASS" > "$SECRETS_DIR/kafka_truststore_creds"

export KAFKA_SSL_TRUSTSTORE_TYPE=JKS
export KAFKA_SSL_TRUSTSTORE_FILENAME="kafka-truststore.jks"
export KAFKA_SSL_TRUSTSTORE_CREDENTIALS="kafka_truststore_creds"
# Do NOT unset KAFKA_SSL_TRUSTSTORE_FILENAME — dub requires it when client auth is enabled
```

Update `kafka-tls.Dockerfile` to COPY from the experiment-13 dockerfiles directory rather
than reusing the experiment-12 entrypoint.

### Guidance for Future Iterations

`dub ensure <VAR>` validates that the named variable is set. The set of required variables
depends on the SSL configuration mode:

| `KAFKA_SSL_CLIENT_AUTH` | `KAFKA_SSL_TRUSTSTORE_FILENAME` required by dub? |
|---|---|
| `none` | No |
| `requested` | Yes |
| `required` | Yes |

When enabling mTLS (`required`) for the first time in a new experiment, create a fresh
entrypoint script rather than reusing one from an experiment that used `none`. Check the
checklist item for EXP-015/EXP-016 and add:

- `KAFKA_SSL_TRUSTSTORE_FILENAME` — bare filename relative to `/etc/kafka/secrets/`
- `KAFKA_SSL_TRUSTSTORE_CREDENTIALS` — credential file for the truststore password

Do not rely on `KAFKA_SSL_TRUSTSTORE_LOCATION` as a substitute — it may be ignored by
dub's validation pass even if Kafka itself would accept it.

---

## EXP-022 — Dockerfile references wrong experiment for reused services (experiment-13, 2026-05-14)

### Symptom

`docker compose up --build` fails with:

```
failed to solve: failed to compute cache key: failed to calculate checksum of ref
...: "/experiments/experiment-10/services/portal-cloud-ml": not found
```

The `portal-cloud-ml` and `service-partner` images cannot be built. The `dynamicorch-xacml`
build (running in parallel) is also canceled as a side-effect of the parallel failure.

### Root Cause

The `portal-cloud-ml.Dockerfile` and `service-partner.Dockerfile` for experiment-13 were
written to reuse source from `experiments/experiment-10/services/portal-cloud-ml/` and
`experiments/experiment-10/services/service-partner/`. Those paths do not exist.

Experiment-10 introduced only `pap` and `pip` services. The `portal-cloud-ml` and
`service-partner` services were introduced in experiment-9 and were not carried forward into
experiment-10's `services/` directory. The Dockerfiles referenced the most recent experiment
number rather than the experiment that actually owns the source.

### Fix

Update both Dockerfiles to reference the correct experiment:

```dockerfile
# portal-cloud-ml.Dockerfile — BEFORE
WORKDIR /build/experiments/experiment-10/services/portal-cloud-ml
COPY experiments/experiment-10/services/portal-cloud-ml/ .

# portal-cloud-ml.Dockerfile — AFTER
WORKDIR /build/experiments/experiment-9/services/portal-cloud-ml
COPY experiments/experiment-9/services/portal-cloud-ml/ .
```

Same change for `service-partner.Dockerfile`.

### Guidance for Future Iterations

When writing a Dockerfile that reuses a service from a previous experiment, check which
experiment **actually contains** that service's `services/<name>/` directory — it is not
always the immediately preceding experiment. A later experiment may have replaced a service
with a different implementation and dropped the old one from its `services/` directory.

Before writing `COPY experiments/experiment-N/services/<name>/`:

```bash
# Find which experiment owns the service source
ls experiments/experiment-*/services/<name>/ 2>/dev/null
```

Use the experiment number returned by that command, not an assumed "latest".

---

## EXP-025 — New dashboard HTML file not served because Dockerfile has explicit COPY list (experiment-13, 2026-05-14)

### Symptom

A new HTML page (`demo.html`) is written to the `dashboard/` directory and the container
is rebuilt with `docker compose up -d --build dashboard`, but the browser still serves the
old content. `docker exec` confirms the file is missing from `/usr/share/nginx/html/`.

### Root Cause

The dashboard `Dockerfile` lists each HTML file individually with explicit `COPY` instructions:

```dockerfile
COPY experiments/experiment-13/dashboard/index.html /usr/share/nginx/html/index.html
COPY experiments/experiment-13/dashboard/admin.html /usr/share/nginx/html/admin.html
# demo.html was never added
```

Adding a file to the source directory does not automatically include it in the image.
Only files with an explicit `COPY` line are present in the built container.

### Fix

Add a `COPY` line for every new HTML file:

```dockerfile
COPY experiments/experiment-13/dashboard/demo.html /usr/share/nginx/html/demo.html
```

Then rebuild: `docker compose up -d --build dashboard`.

### Guidance for Future Iterations

The dashboard Dockerfile uses an explicit per-file COPY pattern (not `COPY dashboard/ /usr/share/nginx/html/`) because the nginx.conf is sourced from a different path. This means **every new HTML file must be manually added** to the Dockerfile.

Whenever a new `.html` file is added to a `dashboard/` directory, immediately check and update the corresponding `dashboard.Dockerfile` — do not wait until the rebuild fails silently.

A fast way to verify before rebuilding:
```bash
grep "COPY.*\.html" experiments/experiment-N/dockerfiles/dashboard.Dockerfile
# Compare against: ls experiments/experiment-N/dashboard/*.html
```

---

## EXP-026 — authzforce-server positional XML parser broken by enriched XACML requests (experiment-13, 2026-05-14)

### Symptom

kafka-authz and topic-auth-xacml log `DENY` for every consumer even after policies
are correctly loaded in the PAP and AuthzForce returns `Permit` when called directly
with a minimal (non-enriched) request.  PIP contains the subject with `certLevel="sy"`.
Direct enriched test:

```bash
# WITHOUT cert-level attributes → Permit ✓
curl -s -X POST "http://localhost:8696/authzforce-ce/domains/$DOMAIN/pdp" ... → Permit

# WITH cert-level="sy" + cert-valid=true → Deny ✗
curl -s -X POST "http://localhost:8696/authzforce-ce/domains/$DOMAIN/pdp" ... → Deny
```

The difference is that the enriched request adds two extra `<Attribute>` elements
(`cert-level`, `cert-valid`) in the subject category before `resource-id`.

### Root Cause

`support/authzforce-server/main.go` — `parseXACMLRequest` used a positional regex:

```go
var attrValueRe = regexp.MustCompile(`<AttributeValue[^>]*>([^<]+)</AttributeValue>`)

func parseXACMLRequest(body string) (subject, resource string) {
    matches := attrValueRe.FindAllStringSubmatch(body, -1)
    if len(matches) >= 1 { subject = matches[0][1] }  // first value
    if len(matches) >= 2 { resource = matches[1][1] }  // second value ← WRONG
    return
}
```

When the PEP sends a standard (non-enriched) XACML request, `AttributeValue` order is:
1. `subject-id` → `portal-cloud-ml`
2. `resource-id` → `telemetry`

This happened to match, so policies without PIP enrichment worked.

When the PEP adds cert-level attributes (experiment-13's PEP design), the order becomes:
1. `subject-id` → `portal-cloud-ml`
2. **`cert-level` → `sy`**   ← positional parser reads this as resource
3. **`cert-valid` → `true`**
4. `resource-id` → `telemetry`

`resource` was set to `"sy"`, and the grant lookup `grants[{"portal-cloud-ml","sy"}]`
returned false → Deny for every enriched request.

The bug was latent in the parser since it was first written; it only became visible when
experiment-13 introduced enriched XACML requests with extra subject attributes.

### Fix

Replace positional extraction with named-attribute extraction, matching on the
`AttributeId` of each `<Attribute>` element:

```go
var namedAttrRe = regexp.MustCompile(
    `<Attribute[^>]+AttributeId="([^"]+)"[^>]*>\s*<AttributeValue[^>]*>([^<]*)</AttributeValue>`)

func parseXACMLRequest(body string) (subject, resource string) {
    for _, m := range namedAttrRe.FindAllStringSubmatch(body, -1) {
        attrID, value := m[1], m[2]
        switch attrID {
        case "urn:oasis:names:tc:xacml:1.0:subject:subject-id":
            subject = value
        case "urn:oasis:names:tc:xacml:1.0:resource:resource-id":
            resource = value
        }
    }
    return
}
```

Added a new test `TestParseXACMLRequest_enriched` covering the interleaved-attribute case.

### Guidance for Future Iterations

**`authzforce-server`'s grant evaluation is based on (subject, resource) only.
Any extra XACML attributes in the request must be ignored — never parsed by position.**

When extending a PEP to include additional subject attributes (cert-level, cert-valid,
role, etc.) in its XACML requests, immediately test the enriched request against
`authzforce-server` directly:

```bash
# Minimal (no extra attributes) → must Permit
curl -s -X POST "http://localhost:$PORT/authzforce-ce/domains/$DOMAIN/pdp" \
  -d '<Request ...><Attributes Category="subject"><Attribute AttributeId="subject-id">...</Attribute></Attributes>...'

# Enriched (extra attributes) → must also Permit
curl -s -X POST "http://localhost:$PORT/authzforce-ce/domains/$DOMAIN/pdp" \
  -d '<Request ...><Attributes Category="subject"><Attribute AttributeId="subject-id">...</Attribute>
      <Attribute AttributeId="urn:arrowhead:attribute:cert-level">...</Attribute></Attributes>...'
```

If Permit → Deny after adding attributes, the parser is treating extra attributes
as positional values.

Checklist addition: see below.

---

## EXP-027 — pki-rest-authz UPSTREAM_URL pointed to HTTP health port, not HTTPS data port (experiment-13, 2026-05-14)

### Symptom

Service partners receive `404 page not found` on every poll after pki-rest-authz
begins returning `PERMIT`. The pki-rest-authz logs show the correct forward target:

```
[pki-rest-authz] PERMIT consumer="service-partner-1" → http://portal-cloud-ml:9207/telemetry/latest
```

Yet calling `http://portal-cloud-ml:9207/telemetry/latest` returns 404.

### Root Cause

`portal-cloud-ml` exposes two ports:

| Port | Handler | Routes |
|------|---------|--------|
| `9207` (`PORT`) | `makeHTTPHandler` | `/health`, `/stats` only |
| `9294` (`TLS_PORT`) | `makeHTTPSHandler` | `/health`, `/stats`, `/telemetry/latest` |

The docker-compose had:

```yaml
UPSTREAM_URL: "http://portal-cloud-ml:9207"
```

`/telemetry/latest` is registered only in `makeHTTPSHandler`, which listens on the
mTLS port 9294.  The health port 9207 knows nothing about that route and returns 404.

The mismatch was easy to miss because:
1. The port numbering scheme (`PORT` = health, `TLS_PORT` = data) is not visible from
   the docker-compose environment block.
2. pki-rest-authz correctly logged the full URL it was forwarding to, but the 404
   appeared in the service partner log — one hop removed — making the path less obvious.

### Fix

```yaml
UPSTREAM_URL: "https://portal-cloud-ml:9294"
```

pki-rest-authz already builds an mTLS upstream client (`buildMTLSUpstreamClient` with
`ownCert` + `caPool`), so switching the scheme to `https` and the port to 9294 is
sufficient — no code changes required.

### Guidance for Future Iterations

**When a service exposes both a plain HTTP health port and an mTLS data port, the
`UPSTREAM_URL` for any reverse proxy must point to the data port and use `https://`.**

Before writing the `UPSTREAM_URL` value in docker-compose, verify which port exposes
the target endpoint:

```bash
grep -n "HandleFunc\|ListenAndServe\|TLS_PORT\|PORT" \
  experiments/experiment-N/services/<target-service>/*.go
```

Look for `makeHTTPHandler` vs `makeHTTPSHandler` (or equivalent) to identify which
mux registers the endpoint being proxied.  If the target endpoint is in the HTTPS
handler, `UPSTREAM_URL` must use the TLS port.

---

## EXP-028 — `KafkaPrincipalBuilder.configure()` does not override — must also implement `Configurable` (experiment-14, 2026-05-17)

### Symptom

Docker build of the `kafka-principal-builder` Maven project fails during `mvn package`:

```
[ERROR] COMPILATION ERROR :
[ERROR] /build/src/main/java/arrowhead/kafka/ArrowheadPrincipalBuilder.java:[80,5]
        method does not override or implement a method from a supertype
[ERROR] Failed to execute goal ... maven-compiler-plugin:3.12.1:compile ...
```

Line 80 is the `@Override` annotation on the `configure(Map<String, ?> configs)` method.

### Root Cause

`KafkaPrincipalBuilder` is a single-method interface with only `build(AuthenticationContext)`.
It does **not** declare `configure()`. The `configure` lifecycle hook is defined in the
separate `org.apache.kafka.common.Configurable` interface:

```java
// kafka-clients — two separate interfaces
public interface KafkaPrincipalBuilder {
    KafkaPrincipal build(AuthenticationContext context);
}

public interface Configurable {
    void configure(Map<String, ?> configs);
}
```

`ArrowheadPrincipalBuilder` was declared as:

```java
public class ArrowheadPrincipalBuilder implements KafkaPrincipalBuilder {
```

The `@Override` on `configure` then fails compilation because `KafkaPrincipalBuilder`
has no `configure` method to override. The Kafka broker will call `configure()` if and
only if the plugin also implements `Configurable`; without it the method is dead code.

### Fix

Add `Configurable` to the `implements` clause and add the corresponding import:

```java
import org.apache.kafka.common.Configurable;

public class ArrowheadPrincipalBuilder implements KafkaPrincipalBuilder, Configurable {
```

No other changes required. The `configure(Map<String, ?> configs)` method body is
correct; the interface was simply missing.

Also remove the unused import of `BrokerSecurityConfigs` (internal API, not referenced
in the method body):

```java
// remove:
import org.apache.kafka.common.config.internals.BrokerSecurityConfigs;
```

### Guidance for Future Iterations

**Every Kafka broker plugin that needs lifecycle configuration must implement both
`KafkaPrincipalBuilder` (or `Authorizer`, etc.) _and_ `Configurable` if it defines
a `configure()` method.**

The pattern for any configurable Kafka plugin:

```java
import org.apache.kafka.common.Configurable;
import org.apache.kafka.common.security.auth.KafkaPrincipalBuilder;

public class MyPrincipalBuilder implements KafkaPrincipalBuilder, Configurable {
    @Override public void configure(Map<String, ?> configs) { ... }
    @Override public KafkaPrincipal build(AuthenticationContext ctx) { ... }
}
```

When adding `@Override` to any method, ensure the interface that declares that method
appears in the `implements` list — the compiler error "method does not override or
implement a method from a supertype" always means one of these two things:
1. The interface/superclass is missing from the `implements`/`extends` clause, or
2. The method signature does not exactly match the interface declaration.

---

## EXP-029 — `KafkaPrincipalBuilder` in KRaft mode must also implement `KafkaPrincipalSerde` (experiment-14, 2026-05-17)

### Symptom

Kafka container exits at startup (exit code 1) with:

```
Exception in thread "main" java.lang.IllegalArgumentException:
  requirement failed: principal.builder.class must implement KafkaPrincipalSerde
    at scala.Predef$.require(Predef.scala:337)
    at kafka.server.KafkaConfig.validateValues(KafkaConfig.scala:2458)
    ...
```

The error appears during `StorageTool` execution (the KRaft metadata log formatting
step that runs before the broker starts), so the container exits before any listener
is opened.

### Root Cause

In KRaft (ZooKeeper-free) mode, Kafka needs to forward authenticated principal
information across the cluster via the internal metadata replication protocol.
To do this it must be able to **serialize and deserialize** `KafkaPrincipal` objects.

Kafka validates at startup that any class configured as `principal.builder.class`
implements **three** interfaces when running in KRaft mode:

| Interface | Purpose |
|---|---|
| `KafkaPrincipalBuilder` | Build principal from `AuthenticationContext` |
| `Configurable` | Receive broker config before first `build()` call |
| `KafkaPrincipalSerde` | Serialize/deserialize principal for inter-broker transport |

`KafkaPrincipalSerde` was added in Kafka 3.x specifically for KRaft support. The
`ArrowheadPrincipalBuilder` class declared only `KafkaPrincipalBuilder, Configurable`,
omitting `KafkaPrincipalSerde`, which caused the startup assertion failure.

### Fix

Add `KafkaPrincipalSerde` to the `implements` clause and implement its two methods:

```java
import org.apache.kafka.common.security.auth.KafkaPrincipalSerde;
import java.nio.charset.StandardCharsets;

public class ArrowheadPrincipalBuilder
        implements KafkaPrincipalBuilder, KafkaPrincipalSerde, Configurable {

    @Override
    public byte[] serialize(KafkaPrincipal principal) throws KafkaException {
        return (principal.getPrincipalType() + ":" + principal.getName())
                .getBytes(StandardCharsets.UTF_8);
    }

    @Override
    public KafkaPrincipal deserialize(byte[] bytes) throws KafkaException {
        String encoded = new String(bytes, StandardCharsets.UTF_8);
        int colon = encoded.indexOf(':');
        if (colon < 0) throw new KafkaException("Invalid principal encoding: " + encoded);
        return new KafkaPrincipal(encoded.substring(0, colon), encoded.substring(colon + 1));
    }
}
```

The `"type:name"` format matches what Kafka's own `DefaultKafkaPrincipalBuilder` uses.

### Guidance for Future Iterations

**A custom `KafkaPrincipalBuilder` deployed against a KRaft broker must implement
`KafkaPrincipalBuilder`, `Configurable`, and `KafkaPrincipalSerde` — all three.**

ZooKeeper-mode brokers do not require `KafkaPrincipalSerde`, so this error only
surfaces in KRaft stacks. Since `cp-kafka` 7.x (Kafka 3.x) uses KRaft by default,
always include the serde implementation.

Template for any new principal builder plugin:

```java
public class MyPrincipalBuilder
        implements KafkaPrincipalBuilder, KafkaPrincipalSerde, Configurable {

    @Override public void configure(Map<String, ?> configs) { ... }
    @Override public KafkaPrincipal build(AuthenticationContext ctx) { ... }

    @Override
    public byte[] serialize(KafkaPrincipal p) throws KafkaException {
        return (p.getPrincipalType() + ":" + p.getName()).getBytes(StandardCharsets.UTF_8);
    }

    @Override
    public KafkaPrincipal deserialize(byte[] bytes) throws KafkaException {
        String s = new String(bytes, StandardCharsets.UTF_8);
        int i = s.indexOf(':');
        if (i < 0) throw new KafkaException("Bad principal bytes");
        return new KafkaPrincipal(s.substring(0, i), s.substring(i + 1));
    }
}
```

---

## EXP-030 — Dashboard fetch URL double-prefixes service name, causing opaque JSON parse error (experiment-13/14, 2026-05-17)

### Symptom

Clicking a button in the Live Monitor sidebar that calls `/api/pip/pip/attributes/{cn}`
shows:

```
Error: Unexpected non-whitespace character after JSON at position 4 (line 1 column 5)
```

The error appears for every option in the dropdown. No network error or HTTP error code
is visible in the UI.

### Root Cause

The dashboard fetch URL was constructed as:

```javascript
fetch('/api/pip/pip/attributes/' + encodeURIComponent(cn))
```

The nginx proxy for the PIP service is configured as:

```nginx
location /api/pip/ {
    rewrite ^/api/pip/(.*) /$1 break;
    proxy_pass $upstream;  # → pip:9506
}
```

nginx strips `/api/pip/` and forwards the remainder. So `/api/pip/pip/attributes/service-partner-1`
becomes `/pip/attributes/service-partner-1` at the PIP service. The PIP's `http.ServeMux`
only registers `/attributes/` (not `/pip/attributes/`), so it returns Go's default 404 response:

```
404 page not found
```

This plain-text response happens to start with `404` — a syntactically valid JSON number.
`JSON.parse` successfully parses `404`, then encounters a space (allowed as whitespace after
a value), then `p` at position 4, which is an unexpected non-whitespace character. The browser
throws "Unexpected non-whitespace character after JSON at position 4" instead of a clear
"404 Not Found" error.

### Fix

Remove the extra service-name segment from the fetch URL. The pattern is:

```
/api/<svc>/<actual-path>   ← correct
/api/<svc>/<svc>/<actual-path>  ← wrong
```

```javascript
// Before (broken):
fetch('/api/pip/pip/attributes/' + encodeURIComponent(cn))

// After (correct):
fetch('/api/pip/attributes/' + encodeURIComponent(cn))
```

After nginx strips `/api/pip/`, the PIP service receives `/attributes/{cn}` which matches
its registered handler.

### Guidance for Future Iterations

**Dashboard fetch URLs for proxied services must not repeat the service prefix.**

When nginx has `location /api/<svc>/` with `rewrite ^/api/<svc>/(.*) /$1`, the path
forwarded to the upstream is everything *after* `/api/<svc>/`. A URL like
`/api/pip/pip/attributes/x` sends `/pip/attributes/x` to the upstream, not `/attributes/x`.

The misleading error message — "Unexpected non-whitespace character after JSON at position 4"
— is caused by Go's `http.ServeMux` 404 body (`"404 page not found"`) parsing as a number
before failing. This masks the real HTTP 404. Check the nginx rewrite rule and the upstream
service's registered routes whenever a JSON parse error occurs on a fetch to `/api/`.

---

## EXP-031 — Kafdrop not reachable: three compounding root causes (experiment-13/14, 2026-05-18)

### Symptom (stage 1)

Kafdrop is added to docker-compose with a custom entrypoint that writes a `kafka.properties`
file and then starts the service. The container appears to start (no `docker compose ps`
exit code) but `http://localhost:<port>/` returns connection refused. Kafdrop never binds
its HTTP port. `docker logs` shows:

```
Error: Unable to access jarfile /kafdrop.jar
```

### Root Cause 1 — Wrong jar path

The custom entrypoint called:
```sh
exec java ... -jar /kafdrop.jar ...
```

The `obsidiandynamics/kafdrop` image does **not** place the jar at `/kafdrop.jar`. The
actual path is `/kafdrop/kafdrop-<version>.jar` (e.g. `/kafdrop/kafdrop-4.2.0.jar`).
Java exits immediately with "Unable to access jarfile" and the HTTP port is never bound.

The native startup script `/kafdrop.sh` handles this correctly using a glob:
```bash
exec java $ARGS -Dloader.path=/extra-classes -jar /kafdrop*/kafdrop*jar ${CMD_ARGS}
```

**Fix:** delegate to `/kafdrop.sh` via `exec /kafdrop.sh` after setting `CMD_ARGS` in the env.

---

### Symptom (stage 2)

After switching to `exec /kafdrop.sh`, the HTTP server starts but the page hangs for
~30 seconds then shows a FreeMarker template error. Kafka connection attempts log:

```
Bootstrap broker kafka:9092 (id: -1 rack: null) disconnected
```

### Root Cause 2 — Spring ClassPathResource: `--kafka.properties.file` swallowed

`/kafdrop.sh` reads `CMD_ARGS` and passes it as Spring Boot CLI args. The entrypoint used:

```sh
export CMD_ARGS="--kafka.brokerConnect=kafka:9092 --kafka.properties.file=/tmp/kafka.properties ..."
```

Spring Boot binds `--kafka.properties.file` to a `Resource propertiesFile` field via
`@Value("${kafka.properties.file:#{null}}")`. `DefaultResourceLoader.getResource("/tmp/kafka.properties")`
calls `ResourceUtils.toURL()` which throws `MalformedURLException` for bare absolute paths,
falls back to `getResourceByPath()`, and returns a `ClassPathContextResource` — not a
`FileSystemResource`. A classpath lookup for `/tmp/kafka.properties` finds nothing; the
properties file is silently not loaded. Kafka connects as PLAINTEXT; the mTLS broker
closes the connection → `disconnected`.

**Fix:** do NOT use `--kafka.properties.file`. Pass each Kafka property as an individual
`--kafka.properties.<key>=<value>` CLI arg instead. Spring binds these to the `Properties properties`
map in `KafkaConfiguration` as plain strings, which Kafka's AdminClient reads directly.

---

### Symptom (stage 3)

After switching to `--kafka.properties.ssl.truststore.location=...` etc. (JKS keystores
created by keytool from OpenSSL-generated PKCS12), the Kafka broker logs:

```
Failed authentication with /172.20.0.x (SSL handshake failed)
```

Kafdrop's cmdline (`/proc/1/cmdline`) confirms all SSL args are present. The JKS files
exist. SSL IS being initiated — but the handshake fails.

### Root Cause 3 — EC private key in SEC1 format; Java requires PKCS#8

The cert-provisioner issues EC private keys in **SEC1/RFC-5915 format**
(`-----BEGIN EC PRIVATE KEY-----`). Java's `KeyFactory` only accepts **PKCS#8 format**
(`-----BEGIN PRIVATE KEY-----`) when loading a private key from PEM. Passing the SEC1 key
to Kafka's PEM keystore loader causes:

```
java.security.InvalidKeyException: IOException: algid parse error, not a sequence
```

This exception prevents the Kafka AdminClient from initializing, causing Spring to fail
to start its web server → Kafdrop is unreachable (connection refused, not just slow).

Additionally, `--kafka.properties.<key>=<val>` CLI args do **not** reliably reach the
Kafka client: Spring Boot's Map/Properties binder may normalize or lose dotted keys like
`security.protocol` at binding time, meaning `security.protocol=SSL` is never applied and
Kafdrop silently falls back to PLAINTEXT. The Kafka broker then logs "SSL handshake failed"
because it receives non-TLS data on an SSL-only listener.

**Fix:** convert the key with `openssl pkcs8`, pass the config via `/kafdrop.sh`'s native
`KAFKA_PROPERTIES` env var (base64-decoded before Java starts), and use `file://` prefix
so Spring's `DefaultResourceLoader` returns a `FileUrlResource` (not `ClassPathResource`):

```sh
# Convert SEC1 → PKCS#8
openssl pkcs8 -topk8 -nocrypt -in "$CERTS/kafka.key" -out /tmp/kafka-pkcs8.key
cat "$CERTS/kafka.crt" /tmp/kafka-pkcs8.key > /tmp/kafka-client.pem

PROPS=$(printf '%s\n' \
  "security.protocol=SSL" \
  "ssl.truststore.type=PEM" \
  "ssl.truststore.location=${CERTS}/ca.crt" \
  "ssl.keystore.type=PEM" \
  "ssl.keystore.location=/tmp/kafka-client.pem" \
  "ssl.endpoint.identification.algorithm=" \
)
export KAFKA_PROPERTIES=$(printf '%s' "$PROPS" | base64 | tr -d '\n')
export KAFKA_PROPERTIES_FILE=/tmp/kafka.properties

export CMD_ARGS="--kafka.brokerConnect=kafka:9092 --kafka.properties.file=file:///tmp/kafka.properties --server.port=9000"
exec /kafdrop.sh
```

---

### Guidance for Future Iterations

**Four rules that must all hold for Kafdrop with mTLS Kafka:**

1. Always delegate to `/kafdrop.sh` (correct jar glob), never `java -jar /kafdrop.jar`.
2. Never use `--kafka.properties.file=/abs/path` — Spring resolves it as ClassPathResource. Use `file:///abs/path` prefix instead.
3. Never pass SSL config as `--kafka.properties.<key>=<val>` CLI args — Spring's binder may lose dotted keys; use `KAFKA_PROPERTIES` base64 + `KAFKA_PROPERTIES_FILE`.
4. Never use EC private keys directly — convert from SEC1 to PKCS#8 with `openssl pkcs8 -topk8 -nocrypt` before passing to Java.

**Kafka PEM SSL properties split into two groups:**

| Property | Value type |
|---|---|
| `ssl.truststore.location` | File path to CA PEM |
| `ssl.keystore.location` | File path to combined cert+key PEM |
| `ssl.keystore.certificate.chain` | PEM **content** (inline) |
| `ssl.keystore.key` | PEM **content** (inline) |

---

## EXP-032 — `support/policy-sync` (and `topic-auth-sync`, `topic-auth-http`) used stale ConsumerAuth path after core path migration (experiment-9, 2026-05-28)

### Symptom

`policy-sync` container enters an unhealthy retry loop immediately after starting, even though
the ConsumerAuth container is healthy and its health-check passes:

```
[policy-sync] init attempt 1 failed: fetchRules: ConsumerAuth lookup returned 404 — retrying in 5s
[policy-sync] init attempt 2 failed: fetchRules: ConsumerAuth lookup returned 404 — retrying in 5s
...
```

All dependent containers (kafka-authz, topic-auth-xacml, robot-fleet services) fail to start
because they wait on `condition: service_healthy` for policy-sync.

### Root Cause

Step 6 of the AH5 conformance update renamed all ConsumerAuth routes from `/authorization/...`
to `/consumerauthorization/authorization/...`. The core system, e2e tests, experiment Go
services, docker-compose files, and dashboard were all updated.

However, three support modules that call `GET /authorization/lookup` were not updated:

| Module | File |
|--------|------|
| `support/policy-sync` | `sync.go` line 93 |
| `support/topic-auth-sync` | `sync.go` line 107 |
| `support/topic-auth-http` | `sync.go` line 90 |

Each module builds its HTTP request as:
```go
http.NewRequest(http.MethodGet, s.caURL+"/authorization/lookup", nil)
```

ConsumerAuth now returns 404 for `/authorization/lookup` because the route no longer exists.
The failure is silent at build time — the path is a string literal, not a constant from the
core package.

The test mocks for `topic-auth-sync` and `topic-auth-http` used the old path as a routing
key in `http.ServeMux`, so tests passed with the old path and would have failed with the
new path — but since the mocks were also not updated, tests kept passing while runtime broke.

### Fix

Updated all three `sync.go` files to use `/consumerauthorization/authorization/lookup`:
- `support/policy-sync/sync.go`
- `support/topic-auth-sync/sync.go`
- `support/topic-auth-http/sync.go`

Updated the corresponding test mock route registrations:
- `support/topic-auth-sync/sync_test.go` (`mockConsumerAuth.ServeHTTP` path check)
- `support/topic-auth-http/sync_test.go` (`mockCA.handler()` mux registration)

Updated documentation: `support/README.md`, `support/DIAGRAMS.md`.

### Lesson

**When renaming a core HTTP path, also audit support modules.** Support modules call core
APIs as string literals — they are not covered by Go's type system and will not produce
compile errors when paths change. Test mocks that contain the old path as a routing key
will also silently pass after the rename, masking the breakage.

**The pattern to look for:** any `http.NewRequest` or `http.Post` call in `support/` that
contains a path segment from the renamed route.

**Checklist item added** — see pre-flight checklist below.

---

## EXP-033 — All `core.Dockerfile` files failed to build after `modernc.org/sqlite` was added (experiment-13, 2026-05-28)

### Symptom

`docker compose up --build` for experiment-13 (and any experiment that builds core binaries) fails at the Go build step for `serviceregistry`, `authentication`, and `consumerauth`:

```
> [serviceregistry builder 4/4] RUN CGO_ENABLED=0 go build -o /app ./cmd/serviceregistry:
go: go.mod requires go >= 1.25.0 (running go 1.22.12; GOTOOLCHAIN=local)
```

All three core service builds fail with exit code 1.

### Root Cause

Step 9 added `modernc.org/sqlite v1.50.1` as a dependency. That library's own `go.mod` declares `go 1.25.0`, which propagates as the minimum Go version into `core/go.mod`. All `core.Dockerfile` files used `FROM golang:1.22-alpine`, which bundles Go 1.22.12 with `GOTOOLCHAIN=local`. Go 1.22 refuses to build a module whose `go` directive specifies a higher version.

The mismatch was invisible locally because the developer machine runs Go 1.25.0.

### Fix

Updated all 13 `core.Dockerfile` files (`experiments/experiment-{2..14}/dockerfiles/core.Dockerfile`) from `FROM golang:1.22-alpine` to `FROM golang:1.25-alpine`.

### Generalised Lesson

A transitive dependency's `go` directive sets the floor for the entire build. When adding any new dependency with `go get`, check the resulting `go` directive in `core/go.mod`. If it bumped, every `core.Dockerfile` must be updated to match before the next Docker build.

---

## EXP-034 — `setup` container fails because CA grant API changed to provider-centric WHITELIST model (experiment-9, 2026-05-28)

### Symptom

`docker compose up --build` for experiment-9 shows:

```
✘ Container experiment-9-setup-1  service "setup" didn't complete successfully: exit 1
```

All downstream services that depend on `setup` (`policy-sync`, `topic-auth-xacml`, `kafka-authz`, etc.) are therefore never started.

### Root Cause

The `setup` container seeded authorization grants using the old per-consumer request shape:

```sh
-d '{"consumerSystemName":"portal-cloud-ml","providerSystemName":"robot-fleet-site-1","serviceDefinition":"telemetry"}'
```

The ConsumerAuthorization API migrated to a provider-centric WHITELIST model. The new grant body is:

```json
{
  "provider":      "robot-fleet-site-1",
  "targetType":    "SERVICE_DEF",
  "target":        "telemetry",
  "defaultPolicy": { "policyType": "WHITELIST", "policyList": ["portal-cloud-ml"] }
}
```

The old fields are unrecognised; the request is rejected with 400.

Two compounding issues made this worse:

1. **5 grants → 2 policies**: The CA now has exactly ONE policy per `(provider, targetType, target)` triple — its `instanceId` is `PR|LOCAL|<provider>|<targetType>|<target>`. Creating separate grants for service-partner-1 and service-partner-2 against the same `portal-cloud-ml/telemetry-rest` target produces a 409 on the second call. The fix is to consolidate multiple consumers into one WHITELIST grant.

2. **Grep pattern mismatch**: The setup script validated success with `grep -qE '"id":|already exists'`. The new response uses `"instanceId":` (not `"id":`) and the conflict message is "authorization policy already exists". The grep for `"id":` does not match `"instanceId":` because the key is `instanceId` with an uppercase `I`. The pattern must be updated to `'"instanceId":|already exists'`.

The `test-system.sh` Section 14 revocation test had the same staleness problems: it looked up the grant with `"consumerSystemName":"service-partner-1"` (no longer in the response shape), revoked by numeric `id` (now a string `instanceId`), and re-granted with the old format.

### Fix

**`docker-compose.yml` setup service** — replace 5 individual grants with 2 consolidated WHITELIST grants:

```sh
# robot-fleet-site-1 telemetry: portal-cloud-ml and test-probe
curl -s -X POST http://consumerauth:8082/consumerauthorization/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"provider":"robot-fleet-site-1","targetType":"SERVICE_DEF","target":"telemetry","defaultPolicy":{"policyType":"WHITELIST","policyList":["portal-cloud-ml","test-probe"]}}' \
  | grep -qE '"instanceId":|already exists'

# portal-cloud-ml telemetry-rest: service-partner-1, service-partner-2, test-probe
curl -s -X POST http://consumerauth:8082/consumerauthorization/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"provider":"portal-cloud-ml","targetType":"SERVICE_DEF","target":"telemetry-rest","defaultPolicy":{"policyType":"WHITELIST","policyList":["service-partner-1","service-partner-2","test-probe"]}}' \
  | grep -qE '"instanceId":|already exists'
```

**`test-system.sh` Section 14** — look up by `targetNames`, extract `instanceId` (string), URL-encode pipes for the revoke URL, and re-grant using the new format with the full WHITELIST.

### Generalised Lesson

When `core/` changes the ConsumerAuthorization API shape (field names, response structure, idempotency key), every `setup` container curl command and every `test-system.sh` revocation test in every experiment must be updated. The old per-consumer request body is silently unrecognised (returns 400), and grep patterns that match `"id":` will not match `"instanceId":`. After any CA API change, run `grep -rn "consumerSystemName\|providerSystemName\|serviceDefinition" experiments/` to find all stale grant calls.

---

## EXP-036 — policy-sync fails with 405: ConsumerAuth lookup method and response shape changed (experiment-9, 2026-05-28)

### Symptom

`docker compose up --build` shows `policy-sync-1  Error` / unhealthy after ~150s:

```
[policy-sync] init attempt N failed: fetchRules: ConsumerAuth lookup returned 405 — retrying in 5s
```

All services that depend on `policy-sync` (kafka-authz, topic-auth-xacml, robot-fleet) are never created.

### Root Cause

The ConsumerAuthorization API changed in two ways that `support/policy-sync/sync.go` did not track:

**1. HTTP method changed: GET → POST**

`fetchRules` called `GET /consumerauthorization/authorization/lookup`. The endpoint handler now
requires `POST` and rejects `GET` with 405 Method Not Allowed.

**2. Request body now required, and response shape completely changed**

The old `GET /lookup` returned rows in the flat per-consumer model:
```json
{"rules": [{"id": 1, "consumerSystemName": "...", "providerSystemName": "...", "serviceDefinition": "..."}], "count": N}
```

The new `POST /lookup` requires at least one filter (`instanceIds`, `cloudIdentifiers`, or `targetNames`),
which makes it unsuitable for a "fetch all" use case. The correct endpoint for fetching all policies is
`POST /consumerauthorization/authorization/mgmt/query` with an empty `{}` body, which returns:
```json
{"policies": [{"instanceId": "...", "provider": "...", "targetType": "...", "target": "...", "defaultPolicy": {"policyType": "WHITELIST", "policyList": ["consumer-1", ...]}}], "count": N, "totalCount": N}
```

**3. Grant expansion logic changed**

The old model had one row per (consumer, provider, service). The new WHITELIST model has one policy per
(provider, target) with a list of consumers. `policy-sync` must now expand each WHITELIST policy into
one `az.Grant` per consumer:

```go
// Old (flat rows, one grant per row):
grants = append(grants, az.Grant{Consumer: r.ConsumerSystemName, Service: r.ServiceDefinition})

// New (WHITELIST expansion):
for _, consumer := range p.DefaultPolicy.PolicyList {
    grants = append(grants, az.Grant{Consumer: consumer, Service: p.Target, Provider: p.Provider})
}
```

`ALL` and `BLACKLIST` policy types cannot be represented as enumerated XACML subject lists and are
logged and skipped.

### Fix

**`support/policy-sync/sync.go`:**
- Replaced `AuthRule` type with `PolicyDef` + `AuthPolicy` + `LookupResponse` matching the new API
- Changed `fetchRules` → `fetchPolicies`: uses `POST /consumerauthorization/authorization/mgmt/query`
  with `bytes.NewBufferString("{}")` as body and `Content-Type: application/json`
- Updated `sync()` grant compilation: iterates over policies, expands WHITELIST policyList into grants

**`support/policy-sync/sync_test.go`:**
- Updated `mockCA` to handle `POST` (returns 405 on `GET`)
- Replaced `TestSync_threeRules_threePolicies` with `TestSync_whitelistExpanded` (WHITELIST format)
- Added `TestSync_multipleProviders` (two WHITELIST policies)
- Added `TestSync_methodIsPost` to assert the HTTP method

### Generalised Lesson

`support/policy-sync` is tightly coupled to the ConsumerAuthorization wire format. Whenever `core/`
changes the ConsumerAuth lookup endpoint (method, path, request body, or response shape), `sync.go`
must be updated in lockstep. The checklist item for EXP-032 ("grep for old path in support/") should be
extended to also cover method changes. Run `go test ./...` in `support/policy-sync` after any such
change — the mock CA in `sync_test.go` will immediately reveal method or shape mismatches.

---

## EXP-035 — Robot-fleet sites show DOWN: missing identity registration in Authentication service (experiment-13, 2026-05-28)

### Symptom

All three `robot-fleet-site-{1,2,3}` dashboard cards show **DOWN** with no stats.
All other services (profile-ca, AuthzForce, core systems, AMQP/Kafka PEPs, portal-cloud-ml, service partners) show OK.
The robot-fleet containers crash-loop: `docker compose logs robot-fleet-site-1` shows:

```
[robot-fleet-tls] authentication failed: auth login returned 401: {"error":"invalid credentials"}
```

followed by an immediate exit.

### Root Cause

The Authentication core system (`cmd/authentication/main.go`) always calls `NewAuthServiceFull`, which wires
an `IdentityRepository`. When `identityRepo != nil`, `Login` performs:

```go
id, ok := s.identityRepo.Get(req.SystemName)
if !ok {
    return nil, ErrInvalidCredentials  // → 401
}
```

If the system is not in the identity store, login fails with 401 regardless of what credentials are supplied.
`robot-fleet-tls` calls `log.Fatalf` on any authentication error.

The `setup` container seeded PAP policies but never called
`POST /authentication/mgmt/identities` to register the robot-fleet systems.
The Authentication service starts with only the bootstrap Sysop identity.

**Compounding issue:** `handleStats` and `handleConfig` GET in `robot-fleet-tls/main.go` dereference
`fleet` without nil-checking it:

```go
func handleStats(w http.ResponseWriter, _ *http.Request) {
    fleetMu.RLock()
    stats := fleet.Stats()   // ← panics if fleet == nil (set only after successful startup)
    fleetMu.RUnlock()
```

The panic is recovered by Go's HTTP server, but `fleetMu.RUnlock()` is never reached — the reader count
leaks. In practice the crash-loop exits before reaching the fleet initialisation anyway, so the nil
dereference is never hit during the crash loop — but it would trigger on every `/stats` or GET `/config`
request during the restart window.

### Fix

**`experiments/experiment-13/docker-compose.yml` setup service:**

1. Add `authentication: condition: service_healthy` to `depends_on`.
2. Call `POST /authentication/mgmt/identities` (plain HTTP port 8081, no mTLS needed from init container)
   before any PAP policy seeding:

```sh
curl -s -X POST http://authentication:8081/authentication/mgmt/identities \
  -H 'Content-Type: application/json' \
  -d '{"authenticationMethod":"PASSWORD","identities":[
    {"systemName":"robot-fleet-site-1","credentials":{"password":"fleet-secret"}},
    {"systemName":"robot-fleet-site-2","credentials":{"password":"fleet-secret"}},
    {"systemName":"robot-fleet-site-3","credentials":{"password":"fleet-secret"}}
  ]}' | grep -q '"identities"'
```

**`experiments/experiment-13/services/robot-fleet-tls/main.go`:**

Add nil guards matching `handleHealth`'s pattern:

```go
func handleStats(w http.ResponseWriter, _ *http.Request) {
    fleetMu.RLock()
    var stats FleetStats
    if fleet != nil {
        stats = fleet.Stats()
    }
    fleetMu.RUnlock()
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(stats)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        fleetMu.RLock()
        var cfg FleetConfig
        if fleet != nil {
            cfg = fleet.Config()
        }
        fleetMu.RUnlock()
        ...
```

### Generalised Lesson

Any service that calls `POST /authentication/identity/login` requires its identity to exist in the
Authentication service's `IdentityRepository`. The repository is empty on first boot (except for Sysop).
The `setup` container must:
1. Depend on `authentication: condition: service_healthy`
2. Register all system identities via `POST /authentication/mgmt/identities` before other services start

Additionally: handlers that dereference a lazily-initialised global pointer must nil-check it inside the
lock region. If the method call panics before `mu.RUnlock()`, Go's HTTP server recovers the panic but the
reader count leaks permanently — subsequent writers (`Lock()`) block forever, and eventually new readers
(`RLock()`) block too (Go's `sync.RWMutex` queues new readers behind a pending writer).

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
- [ ] Any Dockerfile built `FROM confluentinc/cp-*` uses `microdnf` (not `apt-get`) for any package installs — and checks whether the package (e.g. `openssl`) is already present before adding an install step (EXP-013)
- [ ] Kafka SSL entrypoint scripts use `KAFKA_SSL_KEYSTORE_FILENAME` for the keystore (dub translates it to `ssl.keystore.location`) but `KAFKA_SSL_TRUSTSTORE_LOCATION` (absolute path) for the truststore — dub does NOT translate `KAFKA_SSL_TRUSTSTORE_FILENAME` (EXP-015, EXP-016)
- [ ] Kafka SSL truststore is created with `keytool -importcert -trustcacerts`, NOT with `openssl pkcs12 -nokeys` — openssl does not set the Java-required trusted-key-usage attribute (EXP-016)
- [ ] Entrypoint scripts that write keystores add `rm -f` before recreating files — container restarts preserve the filesystem and `set -e` will exit on "alias already exists" (EXP-016)
- [ ] `KAFKA_INTER_BROKER_LISTENER_NAME` is set in docker-compose.yml when the SSL listener is the only advertised listener (EXP-016)
- [ ] `KAFKA_SSL_ENDPOINT_IDENTIFICATION_ALGORITHM: ""` is set when the CA does not issue SANs — Kafka 2.0+ defaults to HTTPS hostname verification which rejects CN-only certs at startup (EXP-016)
- [ ] Every Go module under `experiments/` has a `go.sum` file committed alongside its `go.mod` — run `go mod tidy` in each module directory and commit both files before building Docker images (EXP-014)
- [ ] For modules that use a `replace` directive pointing to a workspace module (e.g. `arrowhead/core-evol`), verify that `go.sum` includes the *transitive* external deps of the replaced module (e.g. `google.golang.org/grpc`) — run `go mod tidy` even when `go.sum` already exists, since workspace-mode tests may succeed with an incomplete `go.sum` that fails isolated Docker builds (EXP-014 variant, experiment-13)
- [ ] Every service whose endpoints are called by `test-system.sh` has a `ports:` mapping in docker-compose.yml — `test-system.sh` runs on the host and reaches services via `localhost:<port>` (EXP-017)
- [ ] Environment variable names in docker-compose.yml match what the service binary reads (check `grep 'envOr\|os.Getenv' <service>/main.go`) — wrong names are silently ignored when Go defaults match, or cause immediate fatal exit when the var is required with no default (EXP-017)
- [ ] Any service that proxies HTTPS requests to another service uses a custom `*http.Client` with `RootCAs` set to the ephemeral CA pool — never `http.DefaultClient` for mTLS upstreams (EXP-018)
- [ ] `test-system.sh` mTLS curl tests use `--resolve <hostname>:<port>:127.0.0.1 https://<hostname>:<port>/...` — never `https://localhost:<port>` when the server cert has a service-name SAN (EXP-019)
- [ ] `test-system.sh` does not combine `curl -w "%{http_code}"` with `|| echo "000"` — curl already outputs `000` on connection failure; the `||` branch doubles it to `000000` (EXP-019)
- [ ] JSON fields from CA/service responses are extracted with `python3 -c 'import json,sys; print(json.load(sys.stdin)["field"])'` — not with `sed`/`grep` + line-number arithmetic across filtered streams (EXP-019)
- [ ] In every `core.Dockerfile` (and any Dockerfile with an `ARG` used inside a build stage), `ARG <name>` is declared **after** `FROM` — `ARG` before `FROM` is out of scope inside the stage and expands to empty string, causing `no Go files in /src/cmd` (EXP-020)
- [ ] Every `proxy_pass` in a dashboard `nginx.conf` uses the `set`+`rewrite`+`proxy_pass` variable pattern with `resolver 127.0.0.11 valid=5s ipv6=off;` — never a literal hostname (startup crash), never `$request_uri` (wrong path), and `set $upstream` must come BEFORE `rewrite ... break` (break stops subsequent set directives). Test standalone with `docker run` before deploying (EXP-021)
- [ ] The dashboard's `depends_on` lists **only** init/one-shot containers (e.g. `cert-provisioner`) — never application services with deep dependency chains. `condition: service_started` still blocks the dashboard if Docker hasn't started the dependency yet due to its own unsatisfied chain. A static nginx dashboard must start immediately so users can observe service health rather than being blocked by it (EXP-021b)
- [ ] Any Dockerfile that reuses a service from a previous experiment references the experiment that **actually contains** the `services/<name>/` directory — verify with `ls experiments/experiment-*/services/<name>/` before writing the COPY path; a later experiment may have replaced or dropped that service (EXP-022)
- [ ] Every `.html` file in `dashboard/` has a corresponding `COPY` line in `dashboard.Dockerfile` — the Dockerfile uses explicit per-file COPY, not a directory glob; new pages are silently absent from the container until added. Verify with `grep "COPY.*\.html" dashboard.Dockerfile` vs `ls dashboard/*.html` (EXP-025)
- [ ] TCP-only service healthchecks (gRPC, raw TCP) on Alpine containers use `nc -z localhost <port>` — never `printf '' > /dev/tcp/localhost/<port>` which is bash-only and fails silently in Alpine's busybox sh with "can't create /dev/tcp/...: nonexistent directory" (EXP-024)
- [ ] When `KAFKA_SSL_CLIENT_AUTH` is `requested` or `required`, the entrypoint script exports `KAFKA_SSL_TRUSTSTORE_FILENAME` (bare filename, relative to `/etc/kafka/secrets/`) and `KAFKA_SSL_TRUSTSTORE_CREDENTIALS` — do NOT unset `KAFKA_SSL_TRUSTSTORE_FILENAME`; dub validates its presence and fails with "KAFKA_SSL_TRUSTSTORE_FILENAME is required" when client auth is enabled. Create a new entrypoint rather than reusing one written for `none` (EXP-023)
- [ ] After adding extra subject attributes (cert-level, cert-valid, etc.) to XACML requests in a PEP, verify `authzforce-server` returns `Permit` for an enriched request — not just a minimal one. The parser must extract subject/resource by `AttributeId`, not by position; extra attributes before `resource-id` will shift positional indices and silently flip the decision to Deny (EXP-026)
- [ ] When an upstream service exposes both a plain HTTP health port and an mTLS data port, `UPSTREAM_URL` in docker-compose must use `https://` and the TLS port — verify by checking which handler (`makeHTTPHandler` vs `makeHTTPSHandler`) registers the target endpoint (EXP-027)
- [ ] Any Kafka broker plugin that defines a `configure(Map<String, ?> configs)` method must implement **both** `KafkaPrincipalBuilder` (or `Authorizer`, etc.) **and** `org.apache.kafka.common.Configurable` — `KafkaPrincipalBuilder` does not declare `configure`; `@Override` on it causes a compilation error without `Configurable` in the `implements` clause (EXP-028)
- [ ] A custom `KafkaPrincipalBuilder` used in KRaft mode must also implement `KafkaPrincipalSerde` — Kafka validates this at startup and exits with `requirement failed: principal.builder.class must implement KafkaPrincipalSerde` if missing. Serialize as `"type:name"` UTF-8 bytes (EXP-029)
- [ ] Dashboard fetch calls for a proxied service must not repeat the service prefix: `/api/<svc>/<actual-path>`, not `/api/<svc>/<svc>/<actual-path>` — nginx strips `/api/<svc>/` and forwards the remainder, so double-prefixing produces `404 page not found` from Go's `http.ServeMux`. The 404 text (`"404 page not found"`) starts with the number `404`, which parses as valid JSON, causing the browser to throw "Unexpected non-whitespace character after JSON at position 4" rather than a clear 404 error (EXP-030)
- [ ] Never call `java -jar <fixed-path>` in a custom entrypoint for a third-party JVM image — inspect the image's native startup script first (`docker inspect` + `docker run --entrypoint cat`) to find the correct jar path (e.g. Kafdrop uses `/kafdrop*/kafdrop*jar` glob); delegate to the native script via `exec /kafdrop.sh` instead of reimplementing it (EXP-031)
- [ ] Kafdrop mTLS config: (1) convert EC key from SEC1 to PKCS#8 with `openssl pkcs8 -topk8 -nocrypt` (Java rejects SEC1 with "algid parse error"); (2) pass SSL config via `KAFKA_PROPERTIES` base64 env var + `KAFKA_PROPERTIES_FILE` (decoded by `/kafdrop.sh` before Java starts); (3) use `file:///abs/path` prefix on `--kafka.properties.file` so Spring returns FileUrlResource not ClassPathResource; never pass SSL config as `--kafka.properties.<key>=<val>` CLI args (Spring binder loses dotted keys like `security.protocol`) (EXP-031)
- [ ] After any core HTTP path rename, run `grep -rn "/<old-path>" support/` and update every matching `http.NewRequest` / `http.Post` call and test mock in `support/` — path strings are not type-checked and test mocks with the old path silently pass while runtime breaks (EXP-032)
- [ ] When `core/` changes the ConsumerAuth lookup endpoint method (GET→POST), path, required body, or response shape, update `support/policy-sync/sync.go` in lockstep: method, URL (`/mgmt/query` for no-filter fetch-all), type definitions, and WHITELIST expansion in `sync()`. Run `go test ./...` in `support/policy-sync` — the mock CA will catch method and shape mismatches immediately (EXP-036)
- [ ] When adding a Go dependency that itself declares a high `go` directive in its own `go.mod` (e.g. `modernc.org/sqlite v1.50.1` requires `go 1.25.0`), update every `core.Dockerfile` that builds from that module — the Docker builder uses the image's bundled toolchain and `GOTOOLCHAIN=local` blocks builds on a lower version; bump `FROM golang:X.Y-alpine` to match the new minimum (EXP-033)
- [ ] After any CA API change, run `grep -rn "consumerSystemName\|providerSystemName" experiments/` — the CA grant API uses a provider-centric WHITELIST model (`provider`/`targetType`/`target`/`defaultPolicy.policyList`) with ONE policy per `(provider, targetType, target)` triple; old per-consumer fields are silently rejected with 400. Also update grep success patterns from `"id":` to `"instanceId":` (EXP-034)
- [ ] The `setup` container must depend on `authentication: condition: service_healthy` and register every non-Sysop system via `POST /authentication/mgmt/identities` before any downstream service starts. Missing registration causes 401 → `log.Fatalf` → crash loop → DOWN in the dashboard (EXP-035)
- [ ] Handlers that dereference a lazily-initialised global (e.g. `fleet *Fleet` assigned only after all connections succeed) must nil-check it inside the `mu.RLock()` / `mu.Lock()` block — a nil method call panics before the deferred unlock, permanently leaking the lock and eventually deadlocking all later callers (EXP-035)
