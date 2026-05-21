# File Browser

Web file manager based on [filebrowser/filebrowser](https://github.com/filebrowser/filebrowser).

Tests **auto-cert shortcut form** (`https_entrypoint: app`) plus a
persistent volume for the SQLite database.

## Deploy

```bash
flotilla deploy --dry examples/filebrowser   # validate
flotilla deploy      examples/filebrowser   # deploy
```

## Files

| file | purpose |
|------|---------|
| `project.yml` | flotilla contract; shortcut `https_entrypoint` |
| `compose.yml` | single-service compose with volume |
| `.env.example` | no required env vars (convention placeholder) |
