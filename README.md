# pocketkv

Simple minimal self hostable kv datastore you can use to serve dynamic read-only values to static sites.

- **Dashboard** (HTTP Basic Auth): create stores, edit JSON values in a code editor, create read-only API keys.
- **API** (read-only, CORS `*`): read one value or all values of a store with an API key. Keys are safe to embed in static sites.
- Go standard library only. All data lives in one JSON file.

## Run locally

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
| `PORT`               | `8080`               | host port, Docker only (`deploy.sh`) |

Real environment variables take precedence over `.env`. Back up by copying `DATA_FILE`.

Basic Auth sends the password with every request, so put the dashboard behind HTTPS (e.g. a reverse proxy) if it is reachable from outside your network.

## Deploy with Docker

On any Linux server with [Docker](https://docs.docker.com/engine/install/) installed:

```sh
git clone https://github.com/katistix/pocketkv.git
cd pocketkv
cp .env.example .env
nano .env            # set DASHBOARD_USER and DASHBOARD_PASSWORD
./deploy.sh
```

`deploy.sh` builds the image and starts a container named `pocketkv` in the background. It restarts automatically if it crashes or the server reboots. The dashboard is at `http://<server-ip>:8080`; set `PORT` in `.env` to use another port.

Your data is kept in the Docker volume `pocketkv-data`, so it survives restarts, redeploys and updates. Inside the container, the listen address and data path are fixed, so `ADDR` and `DATA_FILE` in `.env` are ignored.

```sh
docker logs -f pocketkv                                # follow the logs
./deploy.sh                                            # apply .env changes
git pull && ./deploy.sh                                # update to the latest version
docker cp pocketkv:/data/pocketkv.json ./backup.json   # back up all data
```

### HTTPS (recommended outside your LAN)

To reach it from the internet, put a reverse proxy in front. With [Caddy](https://caddyserver.com), which gets a certificate automatically, the whole `/etc/caddy/Caddyfile` is:

```
kv.example.com {
	reverse_proxy localhost:8080
}
```

Then set `PORT=127.0.0.1:8080` in `.env` and run `./deploy.sh` again, so pocketkv is only reachable through Caddy.

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
Dockerfile           image used by deploy.sh
```

The JSON editor is [Ace](https://ace.c9.io), loaded from jsDelivr. If the CDN can't be reached, the editor falls back to a plain textarea.
