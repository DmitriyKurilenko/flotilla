# Miniflux

Minimalist RSS reader based on [miniflux/v2](https://github.com/miniflux/v2).

Tests **multi-service deployment** (Postgres + application), **manual
Traefik labels**, `depends_on` with `condition: service_healthy`, and
required environment variables (lint rule L007).

## Deploy

```bash
flotilla deploy --dry examples/miniflux   # validate
flotilla deploy      examples/miniflux   # deploy
```

## Files

| file | purpose |
|------|---------|
| `project.yml` | flotilla contract; no `https_entrypoint` (manual labels) |
| `compose.yml` | two-service compose with depends_on and healthchecks |
| `.env.example` | required vars: `DOMAIN`, `DB_PASSWORD` |
