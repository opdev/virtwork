# Stage 1: Build the virtwork binary
# Using Debian-based image (glibc) to match UBI runtime.
# Alpine (musl) would produce a binary incompatible with UBI's glibc.
FROM golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Cache dependency downloads in a separate layer
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=1 is required for github.com/mattn/go-sqlite3
RUN CGO_ENABLED=1 go build -o virtwork ./cmd/virtwork

# Stage 2: Runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL io.k8s.display-name="virtwork" \
      io.k8s.description="Creates virtual machines on OpenShift with continuous workloads for metrics generation" \
      io.openshift.tags="virtwork,kubevirt,openshift,workload-generator" \
      summary="virtwork workload generator for OpenShift Virtualization" \
      description="CLI tool that creates VMs on OpenShift clusters with KubeVirt and runs continuous CPU, memory, database, network, and disk I/O workloads" \
      name="virtwork"

# sqlite-libs provides the runtime shared library for the CGO-compiled go-sqlite3
RUN microdnf install -y sqlite-libs && \
    microdnf clean all

# Create data directory for audit database.
# OpenShift runs containers with an arbitrary UID but always GID 0.
# Setting group ownership to 0 and mode 775 ensures writability.
RUN mkdir -p /data && \
    chown 1001:0 /data && \
    chmod 775 /data

COPY --from=builder /build/virtwork /usr/local/bin/virtwork
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER 1001
WORKDIR /data

ENV VIRTWORK_AUDIT_DB=/data/virtwork.db

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
