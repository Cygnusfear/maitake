{
  description = "maitake dev shell";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "aarch64-darwin" "x86_64-linux" "aarch64-linux" "x86_64-darwin" ];
      forAll = f: builtins.listToAttrs (map (s: { name = s; value = f s; }) systems);
    in {
      devShells = forAll (system:
        let pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            packages = [ pkgs.go_1_25 pkgs.git ];
          };
        });
    };
}
