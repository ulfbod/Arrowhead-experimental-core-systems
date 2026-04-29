# Repository Rules (Authoritative)

This file defines how to work with this repository.

## Source of Truth (Priority Order)

1. SPEC.md → API contract and behavior
2. TEST_PLAN.md → defines correctness
3. EXAMPLES.md → expected usage and formats
4. CLAUDE.md → implementation rules and constraints

## Core Rules

- SPEC.md MUST be followed exactly
- Do NOT invent or simplify API behavior
- All implementations must pass TEST_PLAN.md
- Use EXAMPLES.md to validate request/response formats

## Architecture Rules

- /core → strict, spec-compliant implementation (Go backend + React dashboard)
- /experiments → external usage only (via HTTP)

- NEVER modify core Go packages for experimental purposes

## Dashboard Rules

`core/dashboard/` is part of the core system. It is allowed and governed by these rules:

- The dashboard is a React + TypeScript browser application
- It communicates with the registry via HTTP only (spec-defined API)
- It MUST NOT bypass core logic or import Go packages
- It MUST NOT introduce non-spec-compliant API behavior
- Building the dashboard is optional; `go build` and `go test` must pass without it

## Instruction for Claude

Always read and apply this file before performing any task.
