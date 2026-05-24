#!/bin/bash
# Build script for virtwork golden container disk image
# Copyright 2026 Red Hat
# SPDX-License-Identifier: Apache-2.0
#
# Two-stage build:
#   1. image-builder produces a Fedora qcow2 with blueprint packages
#   2. podman packages the qcow2 as a KubeVirt containerdisk OCI image
#
# Prerequisites:
#   - image-builder  (dnf install image-builder)
#   - podman

set -euo pipefail

REGISTRY="${REGISTRY:-quay.io/opdev}"
IMAGE_NAME="${IMAGE_NAME:-virtwork-disk}"
TAG="${TAG:-latest}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"
DISTRO="${DISTRO:-fedora-42}"
IMAGE_TYPE="${IMAGE_TYPE:-generic-qcow2}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- prerequisite checks ---------------------------------------------------

for cmd in image-builder podman; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "ERROR: $cmd is required but not found on PATH" >&2
    echo "       Install with: sudo dnf install $cmd" >&2
    exit 1
  fi
done

# image-builder may need root for loopback device access
if [[ $(id -u) -ne 0 ]]; then
  echo "image-builder requires root privileges for disk image assembly."
  echo "Re-executing with sudo..."
  exec sudo \
    REGISTRY="$REGISTRY" \
    IMAGE_NAME="$IMAGE_NAME" \
    TAG="$TAG" \
    DISTRO="$DISTRO" \
    IMAGE_TYPE="$IMAGE_TYPE" \
    PUSH="${PUSH:-false}" \
    "$0" "$@"
fi

# --- stage 1: build qcow2 with image-builder --------------------------------

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Building golden disk image: ${FULL_IMAGE}"
echo "======================================================"
echo "Distro:    ${DISTRO}"
echo "Type:      ${IMAGE_TYPE}"
echo "Blueprint: ${SCRIPT_DIR}/blueprint.toml"
echo "Output:    ${TMPDIR}"
echo ""

image-builder build "$IMAGE_TYPE" \
  --distro "$DISTRO" \
  --blueprint "${SCRIPT_DIR}/blueprint.toml" \
  --output-dir "$TMPDIR"

QCOW2="$(find "$TMPDIR" -name '*.qcow2' -print -quit)"
if [[ -z "$QCOW2" ]]; then
  echo "ERROR: image-builder produced no qcow2 file" >&2
  exit 1
fi

echo ""
echo "qcow2 produced: $(basename "$QCOW2") ($(du -h "$QCOW2" | cut -f1))"

# --- stage 2: package as containerdisk OCI image ----------------------------

echo ""
echo "Packaging as containerdisk..."

cp "$QCOW2" "${TMPDIR}/disk.qcow2"

podman build \
  --tag "${FULL_IMAGE}" \
  --file "${SCRIPT_DIR}/Containerfile" \
  "$TMPDIR"

echo ""
echo "Image built successfully: ${FULL_IMAGE}"

# --- optional push -----------------------------------------------------------

if [[ "${PUSH:-false}" == "true" ]]; then
  echo ""
  echo "Pushing image to registry..."
  podman push "${FULL_IMAGE}"
  echo "Image pushed successfully"
fi

echo ""
echo "======================================================"
echo "Build complete!"
echo "Image: ${FULL_IMAGE}"
echo ""
echo "To push this image to the registry, run:"
echo "  PUSH=true ./build.sh"
echo ""
echo "To use a custom registry, run:"
echo "  REGISTRY=my.registry.io ./build.sh"
