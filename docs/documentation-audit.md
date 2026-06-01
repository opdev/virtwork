# Documentation Audit Runbook

This runbook describes how to check whether the project documentation is in sync with the codebase. Run it after any structural change — new packages, renamed interfaces, changed method signatures, build system changes, or dependency migrations.

## When to Run

- After adding, removing, or renaming an `internal/` package
- After changing a public interface (`Workload`, `MultiVMWorkload`, `Auditor`, etc.)
- After changing method signatures (adding error returns, renaming methods)
- After modifying the build system (Dockerfile, go.mod Go version, CGO changes)
- After adding or removing CLI flags
- After changing the orchestration flow (who calls what)
- Periodically as a hygiene check

## Documents to Audit

| Document | What it claims | What to cross-reference |
|----------|---------------|------------------------|
| `README.md` | Go version, CLI flags, architecture layers, project structure, build instructions | `go.mod`, `Dockerfile`, `internal/config/config.go` (flag definitions), `internal/` directory listing |
| `docs/architecture.md` | Layer diagram, dependency arrows, interface signatures (class diagram), concurrency table, orchestration flowchart, project structure, design decisions | Actual `import` statements, `internal/` directory listing, interface definitions in `workload.go` |
| `docs/guide/README.md` | Go version prerequisite | `go.mod` |
| `docs/guide/01-overview.md` | Workload interface listing, complexity table, MultiVMWorkload description, "Finding Your Way" code pointers | `internal/workloads/workload.go` interfaces, actual file locations |
| `docs/guide/02-deploying-workloads.md` | CLI output format, workload names and VM counts, troubleshooting tips | Actual `--dry-run` output, `workloads.DefaultRegistry()` |
| `docs/guide/03-adding-a-workload.md` | Registry registration steps, interface signatures, code examples, test count references, multi-VM checklist | `internal/workloads/registry.go`, `internal/workloads/workload.go`, actual registry entry count |
| `docs/configuration.md` | Every CLI flag, env var, YAML key | `internal/config/config.go`, `cmd/virtwork/main.go` flag bindings |
| `docs/deployment.md` | Kustomize manifest inventory, image references | `deploy/` directory, `Dockerfile` |
| `docs/development.md` | Build commands, test commands, workload addition steps | `Makefile`, `go.mod`, current test patterns |
| `docs/audit-schema.md` | Table DDL, column names, relationships | `internal/audit/schema.go`, `internal/audit/records.go`, `internal/audit/migrate.go` |

## Cross-Reference Checklist

### Versions and Build

```bash
# Go version — compare against all docs that mention it
grep '^go ' go.mod
grep -rn '1\.\(2[0-9]\|3[0-9]\)' README.md docs/guide/README.md docs/guide/03-adding-a-workload.md docs/development.md

# Dockerfile — check builder image, CGO setting, runtime image
head -3 Dockerfile
grep 'CGO' Dockerfile

# Cross-check README and architecture.md Dockerfile descriptions
grep -n 'Dockerfile' README.md docs/architecture.md
```

### CLI Flags

```bash
# Actual flags defined in code
grep -n 'f\..*(' internal/config/config.go | grep -v '//'

# Flags documented in README
# Compare the two lists manually
```

### Interface Signatures

```bash
# Current Workload interface
sed -n '/type Workload interface/,/^}/p' internal/workloads/workload.go

# Current MultiVMWorkload interface
sed -n '/type MultiVMWorkload interface/,/^}/p' internal/workloads/workload.go

# Check docs for stale method names or signatures
grep -rn 'Roles()' docs/
grep -rn 'DataVolumeTemplates()' docs/
grep -rn 'RoleDistribution' docs/
```

### Package Structure

```bash
# Actual internal packages
ls internal/

# Packages listed in docs
grep -n 'internal/' README.md docs/architecture.md | grep -v test | head -30

# Check for packages in code but missing from docs
diff <(ls internal/ | sort) <(grep -ohP 'internal/\K[a-z]+' docs/architecture.md | sort -u)
```

### Registry and Workload Count

```bash
# Actual workload count
grep -c 'func(cfg config.WorkloadConfig' internal/workloads/registry.go

# Counts mentioned in docs
grep -rn 'Len([0-9]\+)\|HaveLen([0-9]\+)\|nine\|eleven\| [0-9]* workload' docs/ README.md
```

### Orchestration Flow

```bash
# Where does orchestration actually live?
grep -rn 'errgroup' internal/ cmd/ --include='*.go' -l

# What does main.go delegate to?
grep -n 'orchestrator\.\|NewRunOrchestrator\|NewCleanupOrchestrator' cmd/virtwork/main.go

# Check if docs still attribute orchestration to main.go
grep -rn 'cmd/virtwork/main.go.*orchestrat' docs/
grep -rn 'namespaceDataVolumes\|NamespaceDataVolumes' docs/
```

### Design Decisions Table (architecture.md)

```bash
# Check each file path reference in the table is still valid
grep -oP '`[a-z/_.]+`' docs/architecture.md | sort -u | while read -r ref; do
  path=$(echo "$ref" | tr -d '`')
  [ -e "$path" ] || echo "MISSING: $path"
done
```

## Common Drift Patterns

These are the changes most likely to cause documentation drift, based on project history:

| Change type | Docs most likely to drift |
|------------|--------------------------|
| New `internal/` package | `README.md` project structure, `docs/architecture.md` layer diagram + project structure |
| Interface method added/renamed/signature changed | `docs/architecture.md` class diagram, `docs/guide/01-overview.md` interface listing, `docs/guide/03-adding-a-workload.md` tutorial + checklist |
| New CLI flag | `README.md` flags table, `docs/configuration.md` |
| New workload added to registry | `README.md` workload table + VM count, `docs/guide/02-deploying-workloads.md` full-suite table, `docs/guide/03-adding-a-workload.md` registry count |
| Build system change (Go version, CGO, base image) | `README.md` prerequisites + build section, `docs/architecture.md` Dockerfile entry, `docs/guide/README.md`, `docs/guide/03-adding-a-workload.md` prerequisites |
| Orchestration refactor (who calls what) | `docs/architecture.md` flowcharts + concurrency table + dependency diagram, `docs/guide/01-overview.md` code pointer table |
| Audit schema change | `docs/audit-schema.md`, `docs/architecture.md` project structure |

## Workflow

1. Run the cross-reference checks above
2. For each discrepancy, categorize as **string replacement** (version, name, path) or **mental model** (flow changed, interface semantics changed, responsibility moved)
3. File a documentation issue using the `04-documentation.yml` template with both categories listed
4. Fix on a branch named `docs/<short-description>-<issue-number>`
5. Commit with `docs:` conventional commit prefix
