# pocketkv

Simple minimal self hostable kv datastore you can use to serve dynamic read-only values to static sites.

- **Dashboard** (HTTP Basic Auth): create stores, edit JSON values in a code editor, create read-only API keys.
- **API** (read-only, CORS `*`): read one value or all values of a store with an API key. Keys are safe to embed in static sites.
- Go standard library only. All data lives in one JSON file.

## Run

```sh
cp .env.example .env   # set DASHBOARD_USER / DASHBOARD_PASSWORD
go run ./cmd/pocketkv
```

Open http://localhost:8080.

| Variable             | Default              |                                  |
|----------------------|----------------------|----------------------------------|
| `DASHBOARD_USER`     | required             | dashboard login                  |
| `DASHBOARD_PASSWORD` | required             | dashboard password               |
| `ADDR`               | `:8080`              | listen address                   |
| `DATA_FILE`          | `data/pocketkv.json` | where everything is stored       |

Real environment variables take precedence over `.env`. Back up by copying `DATA_FILE`.

Basic Auth sends the password with every request, so put the dashboard behind HTTPS (e.g. a reverse proxy) if it is reachable from outside your network.

## API

The OpenAPI spec is served at `/openapi.yaml` (source: [`internal/api/openapi.yaml`](internal/api/openapi.yaml)).

```js
const headers = { Authorization: "Bearer pkv_..." };
// one value
const hero = await fetch("https://kv.example.com/api/v1/values/hero", { headers }).then(r => r.json());
// all values in the store, as { key: value }
const all = await fetch("https://kv.example.com/api/v1/values", { headers }).then(r => r.json());
```

The API key is sent as a bearer token: `Authorization: Bearer <key>`.

Values must be valid JSON (any type: object, array, string, number, …). Keys use letters, digits and `. _ : -`.

## Layout

```
cmd/pocketkv/        main: config, routing, graceful shutdown
internal/config/     .env + environment loading
internal/store/      stores, values, API keys; persisted to one JSON file
internal/api/        public read-only API + openapi.yaml
internal/web/        dashboard handlers + html/template pages
```

The JSON editor is [Ace](https://ace.c9.io), loaded from jsDelivr. If the CDN can't be reached, the editor falls back to a plain textarea.
