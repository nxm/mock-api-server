## Use

```
inputs = {
  mock-api-server.url = "github:nxm/mock-api-server";
};
```

```
services.mock-api-server = {
    enable = true;
    port = 8080;
    host = "0.0.0.0";
    openFirewall = true; 
};
```