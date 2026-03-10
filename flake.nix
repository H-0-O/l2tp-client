{
  description = "L2TP Client - A user-friendly L2TP command-line client for Linux";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Build the Go module
        l2tp-client = pkgs.buildGoModule rec {
          pname = "l2tp-client";
          version = "0.1.0";

          src = ./.;

          vendorHash = null; # To get the correct hash: nix build .#l2tp-client 2>&1 | grep "got:" | cut -d'"' -f2

          # Build from the cmd directory
          subPackages = [ "cmd/l2tp-client" ];

          # Build metadata
          ldflags = [
            "-X main.version=${version}"
            "-X main.buildTime=${self.lastModifiedDate}"
            "-X main.gitCommit=${self.rev or "dirty"}"
          ];

          meta = with pkgs.lib; {
            description = "A user-friendly L2TP command-line client for Linux systems";
            homepage = "https://github.com/H-0-O/l2tp-client";
            license = licenses.mit;
            maintainers = [ ];
            platforms = platforms.linux;
          };
        };
      in
      {
        packages = {
          default = l2tp-client;
          l2tp-client = l2tp-client;
        };

        apps = {
          default = flake-utils.lib.mkApp {
            drv = l2tp-client;
            name = "l2tp-client";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_21
            gopls
            go-tools
            golangci-lint
          ];

          shellHook = ''
            echo "L2TP Client development environment"
            echo "Available commands:"
            echo "  make build    - Build the binary"
            echo "  make test     - Run tests"
            echo "  nix build     - Build with Nix"
            echo "  nix run       - Run the application"
          '';
        };
      });
}