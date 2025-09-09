{
  description = "roxie - Advanced Cluster Security Deployment Tool";

  nixConfig.warn-dirty = false;

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        pythonEnv = pkgs.python311.withPackages (ps: with ps; [
          rich
          pyyaml
          pytest
          pytest-mock
          python-dotenv
        ]);
      in
      {
        devShells.default = pkgs.mkShell {
          name = "roxie-dev";
          
          buildInputs = with pkgs; [
            # Python environment with packages
            pythonEnv
            
            # Kubernetes and container tools
            podman
            skopeo
            kubectl
            kubernetes-helm
            
            # Build tools
            gnumake
            
            # Python development tools
            ruff
            mypy
            
            # Additional useful tools for development
            git
            which
            tokei
          ];
          
          shellHook = ''
            # Only print banner in interactive shells to avoid noise when running scripts (e.g., ./roxie --help)
            if [ -n "$PS1" ] && [ -t 1 ]; then
              echo "🚀 Welcome to the roxie development environment!"
              echo ""
              echo "Available tools:"
              echo "  Python: $(python --version)"
              echo "  kubectl: $(kubectl version --client --short 2>/dev/null || echo 'kubectl available')"
              echo "  helm: $(helm version --short 2>/dev/null || echo 'helm available')"
              echo "  podman: $(podman --version 2>/dev/null || echo 'podman available')"
              echo "  skopeo: $(skopeo --version 2>/dev/null || echo 'skopeo available')"
              echo "  make: $(make --version | head -1)"
              echo "  ruff: $(ruff --version)"
              echo "  mypy: $(mypy --version)"
              echo "  git: $(git --version)"
              echo ""
              echo "To run roxie: ./roxie deploy --help"
              echo "To lint: ruff check ."
              echo "To format: ruff format ."
              echo "To type check: mypy *.py --ignore-missing-imports"
              echo ""
            fi
          '';
          
          # Set environment variables
          PYTHONPATH = ".";
        };
      });
} 