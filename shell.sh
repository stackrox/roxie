#!/usr/bin/env bash
# Enter the roxie Nix development environment

# Fast path when already in the Nix dev environment
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -n "${ROXIE_DEV_SHELL:-}" ]]; then
    echo "🔄 Already in roxie development environment..."
    exit 0
fi

echo "🔄 Entering roxie development environment..."

if command -v nix >/dev/null 2>&1; then
    ROXIE_USER_SHELL="${SHELL:-/bin/sh}"
    exec nix --extra-experimental-features 'nix-command flakes' develop "${REPO_ROOT}" -c \
      env \
        SHELL="$ROXIE_USER_SHELL" \
        ROXIE_USER_SHELL="${ROXIE_USER_SHELL}" \
        ROXIE_DEV_SHELL="1" \
      "$ROXIE_USER_SHELL"
else
    echo "❌ Nix is not installed. Please install Nix first:"
    echo "   curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install"
    echo ""
    echo "Or use the legacy installer:"
    echo "   sh <(curl -L https://nixos.org/nix/install) --daemon"
    exit 1
fi 