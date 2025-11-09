# Contributing to Lumen

Thanks for your interest in contributing! This project is community-driven and welcomes high-quality PRs.

## How to Propose Changes
- **Issues:** Use GitHub Issues for bugs and feature requests. Include steps to reproduce, logs, and expected vs. actual behavior.
- **Pull Requests:** Open PRs against `main`. Keep PRs focused and well-scoped.

## Code Quality Gates
Before pushing:
```bash
go mod tidy
go vet ./...
go test ./... -count=1
make preflight           # if available; runs the focused checks
make lint                # golangci-lint
make staticcheck
make vuln-tools && make vulncheck
```

Release binary safety check (PQC guard):

```bash
go build -trimpath -buildvcs=false -o ./build/lumend ./cmd/lumend
LC_ALL=C strings ./build/lumend | grep -qiE '(pqc_testonly|\bnoop\b.*pqc)' && echo BAD || echo OK
```

## Style & Conventions
- Go 1.25.x recommended. Keep go.mod tidy.
- Keep modules cohesive; avoid leaking test scaffolding into production code.
- Prefer small, readable commits; reference Issue/PR numbers in messages when relevant.

## Communication
We currently use GitHub Issues and Pull Requests only (no external support channel at this time).
