{
  description = "Development environment for etcd-snapshot-driver CSI driver";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          name = "etcd-snapshot-driver-dev";

          buildInputs = with pkgs; [
            # Core development tools
            go_1_25
            gnumake
            git

            # Container and Kubernetes tools
            docker
            kubectl
            kubernetes-helm
            kind

            # Code quality tools
            golangci-lint
            gotools  # includes goimports

            # Optional ETCD tools
            etcd

            # Additional utilities
            curl
            jq
          ];

          shellHook = ''
            echo "etcd-snapshot-driver development environment"
            echo "Go version: $(go version)"
            echo "kubectl version: $(kubectl version --client --short 2>/dev/null || echo 'kubectl not configured')"
            echo "Docker version: $(docker --version 2>/dev/null || echo 'Docker daemon not running')"
            echo ""
            echo "Available commands:"
            echo "  make build       - Build the driver binary"
            echo "  make test        - Run unit tests"
            echo "  make docker-build - Build Docker image"
            echo "  make lint        - Run linters"
            echo ""

            # Set Go environment
            export GOPATH="$HOME/go"
            export PATH="$GOPATH/bin:$PATH"

            # Ensure bin directory exists
            mkdir -p bin
          '';
        };
      }
    );
}
