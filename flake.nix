{
  description = "git-crypt-agessh";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }:
    utils.lib.eachDefaultSystem
      (system:
        with import nixpkgs { inherit system; }; rec {
          packages.git-crypt-agessh = buildGoModule rec {
            pname = "git-crypt-agessh";
            name = pname;
            version = "0.1.0";
            src = ./.;
            vendorSha256 = "kxPxzVsn3bm5FNXFirEjUT5Sq/SLs+anhDBeyX63Vj0=";
          };
          defaultPackage = packages.git-crypt-agessh;

          devShell = mkShell { nativeBuildInputs = [ go gopls ]; };
        }) // {
      overlay = (final: prev: { git-crypt-agessh = self.defaultPackage."${prev.system}"; });
    };
}
