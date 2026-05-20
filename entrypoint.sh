#!/bin/sh
set -e

# VIRTWORK_ARGS is intentionally left unquoted to allow space-delimited
# tokenization (e.g., "--workloads cpu,memory --vm-count 2").
# This means argument values CANNOT contain spaces.
# Do not pass: VIRTWORK_ARGS='--namespace "my namespace"'
# Instead use: VIRTWORK_ARGS='--namespace my-namespace'

if [ -n "$VIRTWORK_COMMAND" ]; then
    # Validate command
    case "$VIRTWORK_COMMAND" in
        run|cleanup)
            echo "[entrypoint] Executing: virtwork $VIRTWORK_COMMAND $VIRTWORK_ARGS"
            # shellcheck disable=SC2086
            exec /usr/local/bin/virtwork "$VIRTWORK_COMMAND" $VIRTWORK_ARGS
            ;;
        *)
            echo "[entrypoint] ERROR: Invalid VIRTWORK_COMMAND='$VIRTWORK_COMMAND'. Must be 'run' or 'cleanup'." >&2
            exit 1
            ;;
    esac
else
    echo "[entrypoint] No VIRTWORK_COMMAND set. Pod will sleep. Use 'oc exec' to run virtwork manually."
    exec sleep infinity
fi
