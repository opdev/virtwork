#!/bin/bash
# Build script for virtwork golden container disk image
# Copyright 2026 Red Hat
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

REGISTRY="${REGISTRY:-quay.io/opdev}"
IMAGE_NAME="${IMAGE_NAME:-virtwork-disk}"
TAG="${TAG:-latest}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "Building golden container disk image: ${FULL_IMAGE}"
echo "======================================================"

# Build the image
podman build \
  --tag "${FULL_IMAGE}" \
  --file Containerfile \
  .

echo ""
echo "Image built successfully: ${FULL_IMAGE}"
echo ""

# Optional: push to registry
if [[ "${PUSH:-false}" == "true" ]]; then
  echo "Pushing image to registry..."
  podman push "${FULL_IMAGE}"
  echo "Image pushed successfully"
  echo ""
fi

# Verify the image contains expected tools
echo "Verifying installed tools..."
echo "=============================="
podman run --rm "${FULL_IMAGE}" /bin/bash -c "
  which stress-ng && echo '✓ stress-ng found' || echo '✗ stress-ng MISSING' &&
  which fio && echo '✓ fio found' || echo '✗ fio MISSING' &&
  which iperf3 && echo '✓ iperf3 found' || echo '✗ iperf3 MISSING' &&
  which pgbench && echo '✓ pgbench found' || echo '✗ pgbench MISSING' &&
  which tc && echo '✓ tc found' || echo '✗ tc MISSING' &&
  which iptables-nft && echo '✓ iptables-nft found' || echo '✗ iptables-nft MISSING'
"

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
