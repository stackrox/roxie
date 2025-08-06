#!/usr/bin/env bash
# Enter the roxie Nix development environment

echo "🔄 Entering roxie development environment..."

if command -v nix >/dev/null 2>&1; then
    exec nix --extra-experimental-features 'nix-command flakes' develop
else
    echo "❌ Nix is not installed. Please install Nix first:"
    echo "   curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install"
    echo ""
    echo "Or use the legacy installer:"
    echo "   sh <(curl -L https://nixos.org/nix/install) --daemon"
    exit 1
fi 