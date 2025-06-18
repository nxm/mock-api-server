{
  description = "Mock API Server - A dynamic API mocking service";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        mockApiServer = pkgs.buildGoModule {
          pname = "mock-api-server";
          version = "1.0.0";

          src = ./.;
          vendorHash = "sha256-JlCZ5sf0+x+YoCna/a03ARoJwwvNdj1fxVGlORVyuzY=";

          meta = with pkgs.lib; {
            description = "Dynamic API mocking server";
            homepage = "https://github.com/nxm/mock-api-server";
            license = licenses.mit;
            maintainers = [ nxm ];
          };
        };
      in
      {
        packages = {
          default = mockApiServer;
          mock-api-server = mockApiServer;
        };

        apps.default = flake-utils.lib.mkApp {
          drv = mockApiServer;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            go-tools
            golangci-lint
          ];
        };
      }
    ) // {
      nixosModules.default = { config, lib, pkgs, ... }:
        with lib;
        let
          cfg = config.services.mock-api-server;
        in
        {
          options.services.mock-api-server = {
            enable = mkEnableOption "Mock API Server";

            port = mkOption {
              type = types.port;
              default = 8080;
              description = "Port on which the mock API server will listen";
            };

            host = mkOption {
              type = types.str;
              default = "127.0.0.1";
              description = "Host address to bind to";
            };

            openFirewall = mkOption {
              type = types.bool;
              default = false;
              description = "Open the firewall for the mock API server port";
            };

            dataDir = mkOption {
              type = types.path;
              default = "/var/lib/mock-api-server";
              description = "Directory to store persistent mock configurations";
            };

            user = mkOption {
              type = types.str;
              default = "mock-api";
              description = "User under which the mock API server runs";
            };

            group = mkOption {
              type = types.str;
              default = "mock-api";
              description = "Group under which the mock API server runs";
            };

            extraArgs = mkOption {
              type = types.listOf types.str;
              default = [];
              description = "Extra command line arguments to pass to the server";
            };
          };

          config = mkIf cfg.enable {
            users.users.${cfg.user} = {
              isSystemUser = true;
              group = cfg.group;
              home = cfg.dataDir;
              createHome = true;
              description = "Mock API Server user";
            };

            users.groups.${cfg.group} = {};

            networking.firewall.allowedTCPPorts = mkIf cfg.openFirewall [ cfg.port ];

            systemd.services.mock-api-server = {
              description = "Mock API Server";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              serviceConfig = {
                Type = "simple";
                User = cfg.user;
                Group = cfg.group;
                ExecStart = "${self.packages.${pkgs.system}.default}/bin/mockapi -host ${cfg.host} -port ${toString cfg.port} ${concatStringsSep " " cfg.extraArgs}";
                Restart = "on-failure";
                RestartSec = "5s";

                NoNewPrivileges = true;
                PrivateTmp = true;
                ProtectSystem = "strict";
                ProtectHome = true;
                ReadWritePaths = [ cfg.dataDir ];

                AmbientCapabilities = mkIf (cfg.port < 1024) [ "CAP_NET_BIND_SERVICE" ];
                CapabilityBoundingSet = mkIf (cfg.port < 1024) [ "CAP_NET_BIND_SERVICE" ];
              };

              environment = {
                HOME = cfg.dataDir;
                MOCK_API_DATA_DIR = cfg.dataDir;
              };
            };
          };
        };
    };
}