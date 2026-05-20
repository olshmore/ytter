#!/usr/bin/env bash
# Install cluster add-ons and ingress
set -euo pipefail
exec "$(dirname "${BASH_SOURCE[0]}")/deploy.sh" addons ingress
