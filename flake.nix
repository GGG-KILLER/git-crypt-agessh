{
  description = "git-crypt-agessh";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }:
    utils.lib.eachDefaultSystem
      (system:
        with import nixpkgs { inherit system; }; {
          packages.default = buildGoModule rec {
            pname = "git-crypt-agessh";
            name = pname;
            version = "0.1.0";
            src = ./.;
            vendorSha256 = "kxPxzVsn3bm5FNXFirEjUT5Sq/SLs+anhDBeyX63Vj0=";
          };

          devShells.default = mkShell { nativeBuildInputs = [ go gopls ]; };
        }) // {
      overlays.default = (final: _: { git-crypt-agessh = self.packages."${final.system}".default; });
    };
}
