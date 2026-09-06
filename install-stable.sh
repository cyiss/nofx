#!/bin/sh
# Compatibility entry point: install the current reviewed checkout, not an upstream image.
set -eu
SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)
exec sh "$SCRIPT_DIR/install.sh" "$@"
