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

        # Version information derived from the flake source; git tags are not
        # visible inside a flake build, so use the commit hash instead of the
        # `git describe` scheme the Makefile uses.
        version = self.shortRev or "dirty";
        buildDate =
          let d = self.lastModifiedDate or "19700101000000";
          in with pkgs.lib.strings;
          "${substring 0 4 d}-${substring 4 2 d}-${substring 6 2 d}T${substring 8 2 d}:${substring 10 2 d}:${substring 12 2 d}Z";

        # Build roxie
        roxie = pkgs.buildGoModule {
          pname = "roxie";
          inherit version;

          src = ./.;

          # Let Nix handle vendoring by calculating the hash
          # To update: set to pkgs.lib.fakeHash, build, then use the hash from error
          vendorHash = "sha256-CpzbXRNw8VYli3ZX6SZa6j3EpkuTRpY4LgNzAT/Qrkw=";

          # Inject version information at build time
          ldflags = [
            "-X main.version=${version}"
            "-X main.buildDate=${buildDate}"
          ];

          subPackages = [ "cmd" ];

          # The main package lives in ./cmd, so the binary is named after
          # that directory; rename it to the actual tool name.
          postInstall = ''
            mv $out/bin/cmd $out/bin/roxie
          '';

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
              echo "  - Go $(go version | cut -d' ' -f3)"
              echo ""
              echo "💡 For full set of pre-installed tooling use:"
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
              k9s
              stern

              # Container tools:
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
