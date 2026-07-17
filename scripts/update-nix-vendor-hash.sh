#!/usr/bin/env bash

# Updates the vendorHash in flake.nix if it no longer matches go.mod/go.sum.
# Runs a nix build, extracts the correct hash from the mismatch error, patches
# flake.nix in place, and rebuilds to verify.
#
# Exits 0 if the hash was already correct or was updated successfully,
# non-zero if the build fails for any other reason.

set -euo pipefail

cd "$(dirname "$0")/.."

log=$(mktemp)
trap 'rm -f "$log"' EXIT

if nix build --no-link '.#roxie' 2>&1 | tee "$log"; then
    echo "vendorHash is up to date."
    exit 0
fi

if ! grep -q "hash mismatch in fixed-output derivation '.*-go-modules" "$log"; then
    echo "nix build failed for a reason other than a stale vendorHash." >&2
    exit 1
fi

new=$(awk '/got:/ { print $2; exit }' "$log")
old=$(sed -n 's/.*vendorHash = "\(sha256-[^"]*\)".*/\1/p' flake.nix)

if [ -z "$new" ] || [ -z "$old" ]; then
    echo "Failed to extract vendorHash (old: '$old', new: '$new')." >&2
    exit 1
fi

sed -i "s|$old|$new|" flake.nix
echo "Updated vendorHash: $old -> $new"

echo "Verifying the build with the new vendorHash..."
nix build --no-link '.#roxie'
