# Virtwork Guide

This guide walks you through how virtwork works, how to deploy workloads with it, and how to extend it with new workload types. It complements the [reference documentation](../../README.md) with narrative explanations and hands-on demos.

## Who This Guide Is For

- **Operators** who want to deploy workload VMs on OpenShift and understand what they produce
- **Developers** who want to add new workload types or contribute to the project

## Prerequisites

- Familiarity with Kubernetes/OpenShift concepts (namespaces, pods, services)
- For the deployment demo: an OpenShift cluster with [OpenShift Virtualization](https://docs.openshift.com/container-platform/latest/virt/about_virt/about-virt.html) (CNV) installed
- For the developer tutorial: Go 1.26+ and the [Ginkgo CLI](https://onsi.github.io/ginkgo/#installing-ginkgo)

## Guide Sections

1. **[How Virtwork Works](01-overview.md)** — Build a mental model of how the pieces fit together by tracing a single run through the system
2. **[Demo: Deploying Workloads](02-deploying-workloads.md)** — Seven hands-on scenarios from dry run to SSH debugging to cleanup
3. **[Tutorial: Building an HTTP Workload](03-adding-a-workload.md)** — Code a new workload type from scratch using TDD

## Reference Documentation

- [README](../../README.md) — Quick start, CLI reference, configuration, audit tracking, OpenShift deployment
- [Architecture](../architecture.md) — Layered architecture diagrams and design decisions
- [Development](../development.md) — Developer environment setup, testing, adding workloads (reference)
