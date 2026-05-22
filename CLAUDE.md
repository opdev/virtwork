# Claude Code Instructions

## Git Policy
- **NEVER push to remote repository** - User handles all pushes manually
- **DO make frequent incremental commits** - Small, focused commits for easy rollback
- Use conventional commit messages (feat:, fix:, test:, docs:, refactor:, chore:)
- DO NOT commit secrets or other sensitive information
- DO NOT commit engineering journals
- Every commit must include a DCO `Signed-off-by` trailer (enforced by `commit-msg` hook); do NOT add a `Co-Authored-By` trailer

## Role Context
- Expert Go developer
- Expert OpenShift administrator
- Expert OpenShift developer

## Development Approach
- TDD: Write tests first
- BDD: Use Ginkgo BDD framework
- Use goroutines and channels for concurrency
- Use mermaid for documentation diagrams
- Use `internal/logging.NewLogger()` for structured logging in any code that performs I/O; do not use `fmt.Fprintf` or `log.Printf`. The pure data layer (`internal/workloads`, `internal/cloudinit`) should not log.

## Testing
- Unit tests: `{source}_test.go` files alongside source
- Integration tests: `{source}_integration_test.go` files alongside source
- E2E tests: `tests/e2e/`
- Run all: `go test ./...`

## Database
- When a database is needed, use SQLite for local testing
- Strictly adhere to PostgreSQL syntax standards (e.g., use standard SQL for dates, avoid loose typing)
- Assume the production DB is strict.

## Documentation Conventions
- `docs/README.md` is the index/TOC for the `docs/` directory; link new docs from there.
- Use mermaid for architecture, flow, sequence, ERD, and topology diagrams.
- Do NOT reference bare issue numbers (`#123`) in published docs — if context from an issue matters, fold the relevant information into the doc prose so readers don't need to chase the tracker.
- Two docs are intentionally frozen as historical snapshots and should not be updated piecemeal: `docs/implementation-plan.md` and `docs/openshift-virtualization-workload-automation.md`. Add new content to the live docs instead (architecture.md, development.md, configuration.md, etc.).
- Engineering journals are not committed to this repo — that is the policy, not a missing directory.