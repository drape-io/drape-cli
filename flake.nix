{
  description = "Drape CLI - upload test results and coverage to Drape";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "drape";
          version = "dev";
          src = ./.;
          # Run: nix build 2>&1 | grep "got:" to get the real hash
          vendorHash = null;

          ldflags = [
            "-s" "-w"
            "-X main.version=dev"
            "-X main.commit=nix"
            "-X main.date=1970-01-01T00:00:00Z"
          ];

          meta = with pkgs.lib; {
            description = "CLI for uploading test results and coverage to Drape";
            homepage = "https://drape.io";
            license = licenses.mpl20;
            mainProgram = "drape";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go gopls golangci-lint ];
        };
      });
}
