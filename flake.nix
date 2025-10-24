{
  description = "roxie - Advanced Cluster Security Deployment Tool";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Get git commit hash (short version - first 8 chars)
        # Uses builtins.readFile to get hash even from dirty repos
        gitCommit =
          let
            shortRev = self.shortRev or null;
            dirtyRev = builtins.readFile (pkgs.runCommand "git-hash" {
              nativeBuildInputs = [ pkgs.git ];
            } ''
              git -C ${./.} rev-parse --short=8 HEAD > $out 2>/dev/null || echo "unknown" > $out
            '');
            cleanDirtyRev = pkgs.lib.strings.removeSuffix "\n" dirtyRev;
          in
            if shortRev != null then shortRev
            else if self ? rev then "${cleanDirtyRev}-dirty"
            else cleanDirtyRev;

        # Build roxie
        roxie = pkgs.buildGoModule {
          pname = "roxie";
          version = "0.1-${gitCommit}";

          src = ./.;

          # Let Nix handle vendoring by calculating the hash
          # To update: set to pkgs.lib.fakeHash, build, then use the hash from error
          vendorHash = "sha256-bIlSwBh8WJtscEtjQIvxdIK9sFR7aQNV2BUeVNj8qbA=";

          # Inject version information at build time
          ldflags = [
            "-X main.version=0.1"
            "-X main.gitCommit=${gitCommit}"
            "-X main.buildDate=1970-01-01T00:00:00Z"
          ];

          subPackages = [ "cmd/roxie" ];

          meta = with pkgs.lib; {
            description = "Fast, developer-friendly CLI to deploy and manage Red Hat Advanced Cluster Security (ACS)";
            homepage = "https://github.com/stackrox/roxie";
            license = licenses.asl20;
            maintainers = [ ];
          };
        };

      in
      {
        # Package outputs
        packages = {
          default = roxie;
          roxie = roxie;
        };

        # Development shell with roxie and essential dependencies
        devShells = {
          # Default: Minimal shell (fast, essential tools only)
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              # Go development tools
              go
              gopls
              gotools
              golangci-lint

              # roxie binary
              roxie

              # Essential Kubernetes tools (lightweight)
              kubectl
              kubernetes-helm

              # Optional: Kubernetes utilities (lightweight)
              k9s
              stern
            ];

            shellHook = ''
              echo "🚀 roxie development environment (minimal)"
              echo ""
              echo "Available tools:"
              echo "  - roxie ($(roxie version))"
              echo "  - kubectl ($(kubectl version --client --short 2>/dev/null || echo 'not configured'))"
              echo "  - helm ($(helm version --short 2>/dev/null || echo 'unknown'))"
              echo "  - Go $(go version | cut -d' ' -f3)"
              echo ""
              echo "💡 For container tools (skopeo, podman), use:"
              echo "   nix develop .#full"
              echo ""
              echo "Run 'roxie --help' to get started"
            '';
          };

          # Full shell with all dependencies (including heavy ones)
          full = pkgs.mkShell {
            buildInputs = with pkgs; [
              # Go development tools
              go
              gopls
              gotools
              golangci-lint

              # roxie binary
              roxie

              # Kubernetes tools
              kubectl
              kubernetes-helm
              k9s
              stern

              # Container tools (heavy dependencies!)
              skopeo
              podman

              # Load balancer
              haproxy
            ];

            shellHook = ''
              echo "🚀 roxie development environment (full)"
              echo ""
              echo "Available tools:"
              echo "  - roxie ($(roxie version))"
              echo "  - kubectl ($(kubectl version --client --short 2>/dev/null || echo 'not configured'))"
              echo "  - helm ($(helm version --short 2>/dev/null || echo 'unknown'))"
              echo "  - skopeo ($(skopeo --version | head -n1))"
              echo "  - podman ($(podman --version | head -n1))"
              echo "  - haproxy ($(haproxy -v | head -n1))"
              echo ""
              echo "Run 'roxie --help' to get started"
            '';
          };
        };

        # App for 'nix run'
        apps.default = {
          type = "app";
          program = "${roxie}/bin/roxie";
        };

        # Formatter for 'nix fmt'
        formatter = pkgs.nixpkgs-fmt;
      }
    );
}
