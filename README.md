# Mock Api Server

![image](https://github.com/user-attachments/assets/4fb9d4e6-1610-43d6-93e3-eadd89ba584e)

## Demo

Try the live demo at: https://mock-api.jakub.app/admin

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
