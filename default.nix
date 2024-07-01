{
  pkgsSrc ? builtins.fetchTarball {
    name = "nixpkgs-24.05";
    url = "https://github.com/NixOS/nixpkgs/archive/5646423bfac84ec68dfc60f2a322e627ef0d6a95.tar.gz";
    sha256 = "sha256:1lr1h35prqkd1mkmzriwlpvxcb34kmhc9dnr48gkm8hh089hifmx";
  },
}: let
    pkgs = (import pkgsSrc) {};
in {
    shell = pkgs.mkShell {
        name = "project-shell";
        buildInputs = [
          pkgs.go
          pkgs.golangci-lint
          pkgs.xcaddy
          pkgs.caddy
        ];
        shellHook = ''
          mkdir -p .dev-home
          echo '*' > .dev-home/.gitignore
          export XDG_CONFIG_HOME=$(pwd)/.dev-home/config
          export XDG_DATA_HOME=$(pwd)/.dev-home/data
        '';
    };
}
