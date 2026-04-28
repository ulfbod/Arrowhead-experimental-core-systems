# CLAUDE.md

## Project Overview

This repository implements the **Arrowhead Service Registry Core System** in Go.

Specification:
- SPEC.md (authoritative, MUST be followed)

---

## Source of Truth (STRICT ORDER)

1. SPEC.md → complete behavioral contract
2. TEST_PLAN.md → defines correctness
3. EXAMPLES.md → clarifies expected behavior

Claude MUST NOT deviate from these.

---

## Critical Rule

Claude MUST implement **ALL fields and behaviors defined in SPEC.md**, including:

- metadata handling
- version handling
- interface matching
- overwrite semantics for duplicates

Partial implementations are NOT allowed.

---

## Language Rules

- Go ONLY
- Prefer standard library
- Minimal dependencies

---

## Architecture

- cmd/
- internal/
  - api/
  - service/
  - repository/
  - model/

---

## Implementation Rules

Claude MUST:

- Implement all endpoints defined in SPEC.md
- Apply ALL validation rules
- Implement full matching logic:
  - serviceDefinition
  - interfaces
  - metadata
  - version

- Ensure deterministic behavior

Claude MUST NOT:

- Skip optional fields defined in SPEC.md
- Simplify matching logic
- Add undefined features

---

## Testing Rules

- Tests MUST reflect TEST_PLAN.md
- All spec features must be tested:
  - metadata
  - version
  - duplicates
  - edge cases

---

## Build Requirements

Must always pass:

- go build
- go test

---

## Workflow

1. Read SPEC.md fully
2. Implement models
3. Implement logic
4. write tests
5. verify build
6. clean code

---

## Summary

This is a **strict, spec-complete implementation**.

Correctness > simplicity.