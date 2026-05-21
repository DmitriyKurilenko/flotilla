# Echo Server

Lightweight HTTP echo server based on [Ealenn/Echo-Server](https://github.com/Ealenn/Echo-Server).

Tests the **explicit `https_entrypoint` object form** (`service` + `port`)
and a compose-native healthcheck with `127.0.0.1`.

## Deploy

```bash
flotilla deploy --dry examples/echo-server   # validate
flotilla deploy      examples/echo-server   # deploy
```

## Files

| file | purpose |
|------|---------|
| `project.yml` | flotilla contract; explicit `https_entrypoint` |
| `compose.yml` | single-service compose; Alpine-based image |
| `.env.example` | no required env vars (convention placeholder) |
