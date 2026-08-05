# Contributing

Contributions are welcome. This guide covers the essentials.

## Reporting Issues

Open a GitHub issue with:
- Which area is affected (`core/`, `support/`, `experiments/`, or `core-evol/`)
- Steps to reproduce (or the experiment number and `docker compose` command used)
- Expected vs. actual behavior

## Submitting Changes

1. Fork the repository and create a feature branch.
2. Make sure your changes build and pass tests:
   ```bash
   cd core && go build ./... && go test ./...
   ```
3. For experiment changes, verify the Docker Compose stack starts and `test-system.sh` passes.
4. Open a pull request with a clear description of what changed and why.

## Area-Specific Rules

The repository has strict separation between areas. Read the relevant guide before contributing:

| Area | Guide | Standard |
|---|---|---|
| `core/` | [core/CLAUDE.md](core/CLAUDE.md), [core/SPEC.md](core/SPEC.md) | Strict — spec-compliant, fully tested |
| `core-evol/` | [core-evol/README.md](core-evol/README.md) | Stable — extends core with ADAPI interfaces |
| `support/` | [support/CLAUDE.md](support/CLAUDE.md) | Stable — production-quality shared libraries |
| `experiments/` | [experiments/CLAUDE_EXPERIMENTS.md](experiments/CLAUDE_EXPERIMENTS.md) | Exploratory — self-contained Docker stacks |

The fundamental boundary rule: **no code outside `core/` may import packages from `core/internal/`**. If an experiment needs something the core does not expose, add it to the core HTTP API.

## Code Style

- Go: standard `gofmt` formatting, `go vet` clean
- TypeScript (dashboards): project-level ESLint and TSConfig
- No hardcoded ports, hostnames, or domain names — use environment variables

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
