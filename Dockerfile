# Stage 1: Build the virtwork binary
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependency downloads in a separate layer
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o virtwork ./cmd/virtwork

# Stage 2: Runtime image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

LABEL io.k8s.display-name="virtwork" \
      io.k8s.description="Creates virtual machines on OpenShift with continuous workloads for metrics generation" \
      io.openshift.tags="virtwork,kubevirt,openshift,workload-generator" \
      summary="virtwork workload generator for OpenShift Virtualization" \
      description="CLI tool that creates VMs on OpenShift clusters with KubeVirt and runs continuous CPU, memory, database, network, and disk I/O workloads" \
      name="virtwork"

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
