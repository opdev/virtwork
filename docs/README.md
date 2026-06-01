# Virtwork Documentation

This is the documentation index for virtwork. Start with the top-level [README](../README.md) for project orientation and quick start; come here when you need a specific reference.

## Start Here

- [../README.md](../README.md) — Project overview, motivation, installation, quick start, CLI reference, configuration summary, OpenShift deployment overview, architecture summary
- [architecture.md](architecture.md) — Layered architecture, mermaid diagrams, concurrency model, workload class diagram, key design decisions

## Guides

Hands-on, narrative walkthroughs for first-time users and new contributors.

- [guide/01-overview.md](guide/01-overview.md) — Mental model: the deploy-and-exit lifecycle, the `Workload` and `MultiVMWorkload` interfaces, where to find things in the code
- [guide/02-deploying-workloads.md](guide/02-deploying-workloads.md) — Nine scenarios from dry-run to chaos to cluster-side deployment, with copy-pasteable commands and expected output
- [guide/03-adding-a-workload.md](guide/03-adding-a-workload.md) — End-to-end TDD walkthrough that builds a new workload from scratch; covers simple, storage-backed, and multi-VM patterns

## Reference

Targeted references — read when you need a specific fact.

- [configuration.md](configuration.md) — Complete reference for every CLI flag, environment variable, YAML key, and ConfigMap key (including per-workload parameters)
- [chaos-workloads.md](chaos-workloads.md) — Operator guide for the three chaos engineering workloads (chaos-disk, chaos-network, chaos-process), including destructive-behavior warnings
- [audit-schema.md](audit-schema.md) — SQLite audit database: five-table ERD, column-by-column reference, common queries, record lifecycle
- [deployment.md](deployment.md) — OpenShift Kustomize deployment deep-dive: resource topology, RBAC scope, ConfigMap/Secret keys, image pinning, sizing, audit-DB persistence
- [virtwork-vs-kube-burner.md](virtwork-vs-kube-burner.md) — Positioning compared to kube-burner; what each tool measures and where they complement each other

## Contributor

- [development.md](development.md) — Environment setup, building, running unit/integration/E2E tests, cluster prerequisites, Makefile targets, adding a new workload, SSH/audit configuration, testing patterns, commit conventions
- [documentation-audit.md](documentation-audit.md) — Runbook for checking documentation against the codebase; cross-reference checklist, common drift patterns, and workflow
- [../build/golden-image/README.md](../build/golden-image/README.md) — Building and using the optional Fedora-based golden container disk image with pre-installed workload tools

## Historical

These documents capture earlier project state and are intentionally not updated. They are preserved as context.

- [implementation-plan.md](implementation-plan.md) — Original phased build plan (phases 0–12), pre-chaos / pre-tps / pre-logging
- [openshift-virtualization-workload-automation.md](openshift-virtualization-workload-automation.md) — Original design rationale, written before any application code existed

---

> **Conventions for this directory**
>
> - Diagrams are written in mermaid so they render natively on GitHub. Update them in place when the code they describe changes.
> - Bare issue numbers (`#123`) are not used in published docs — any context that matters is folded into the prose.
> - The historical snapshots above are frozen on purpose. New content goes into the live docs, not into them.
