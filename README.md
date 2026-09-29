<div align="center">
  <img src="frontend/public/favicon.svg" width="76" alt="Avari Domains logo" />
  <h1>Avari Domains</h1>
  <p><strong>Domain, website, DNS, TLS, and email diagnostics in one clear report.</strong></p>
  <p>
    <a href="https://domains.avari.dev">Live demo</a> ·
    <a href="README_RU.md">Русская версия</a> ·
    <a href="openapi.yaml">API specification</a>
  </p>
  <p>
    <img src="https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white" alt="Go 1.27" />
    <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React 19" />
    <img src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white" alt="TypeScript 5" />
    <img src="https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite&logoColor=white" alt="SQLite" />
    <img src="https://img.shields.io/badge/license-MIT-green.svg" alt="MIT license" />
  </p>
</div>

## See it in action

| Check a domain | Read the diagnostic report |
|:--:|:--:|
| ![Avari Domains landing page](docs/screenshots/home.png) | ![Avari Domains diagnostic report](docs/screenshots/report.svg) |

*Static interface previews. The report illustration demonstrates the layout; live results depend on the domain and current network conditions.*

## What it does

Avari Domains turns a hostname or URL into a practical diagnostic report. It separates the domain used for registration checks from the host used for website checks, and explains each result instead of collapsing every failure into a simple “available” or “unavailable” label.

- **Registration:** checks domain status through RDAP with WHOIS fallback. An inconclusive response stays unknown; it is never presented as proof that a domain is free.
- **Website and DNS:** inspects A, AAAA, CNAME, NS records, delegation consistency, TLS, and HTTP/HTTPS responses.
- **Email posture:** checks MX, SPF, DKIM, and DMARC and presents the findings with plain-language summaries.
- **Registrar comparison:** shows available first-year and renewal pricing, source and freshness details, and links to the registrar. Purchase takes place on the registrar’s site.
- **Browser history:** keeps a paginated history and summary statistics for the current browser session.

## Built with

- **Frontend:** React 19, TypeScript, Vite, TanStack Query, and Framer Motion.
- **Backend:** Go HTTP service with concurrent diagnostics and an OpenAPI 3.1 contract.
- **Storage:** SQLite for cached results and browser-scoped history.
- **Deployment:** Docker Compose, static frontend served by Go, and OpenResty reverse-proxy configuration for VPS deployment.

## Run locally

### Docker Compose

```sh
docker compose up --build
```

Open [http://localhost:8080](http://localhost:8080). SQLite data is stored in a persistent Docker volume. The service health endpoint is `/api/health`.

### Development mode

Start the backend from `backend/`:

```sh
go run .
```

In another terminal, start the frontend from `frontend/`:

```sh
npm install
npm run dev
```

Vite proxies `/api` to `localhost:8080`. For production, the Go service serves the compiled `frontend/dist` directory; set `STATIC_DIR` if the directory is elsewhere.

## Configuration

The app works without registrar credentials. Set these optional environment variables to enable exact-domain pricing checks through the Porkbun API:

| Variable | Purpose |
|---|---|
| `PORKBUN_API_KEY` | Porkbun API key |
| `PORKBUN_SECRET_API_KEY` | Porkbun secret API key |
| `CHECK_REGION` | Region label attached to diagnostic results |
| `DNS_RESOLVER` | DNS resolver used for lookups |
| `SQLITE_PATH` | SQLite database path |
| `LISTEN_ADDR` | HTTP listen address |
| `STATIC_DIR` | Built frontend directory |
| `TRUSTED_PROXY_IPS` | Proxy addresses allowed to provide client IP information |

See `docker-compose.yml` and `backend/main.go` for defaults. Do not commit API credentials or production `.env` files.

## API

The HTTP API is documented in [openapi.yaml](openapi.yaml). Main endpoints:

- `GET /api/health` — service health.
- `POST /api/check` — run or retrieve a cached diagnostic report.
- `GET /api/history?page=0` — list the current browser’s checks.
- `GET /api/history/{id}` — retrieve a saved report belonging to the current browser.
- `GET /api/stats` — summary counts for the current browser.

Requests are rate limited by client IP. Browser history is retained for 90 days; diagnostic results are cached with status-dependent expiration times. Website checks connect only to public IP addresses and validate redirect destinations.

## Project structure

```text
.
├── backend/                 Go API, diagnostics, SQLite store
├── frontend/                React + TypeScript application
├── docs/screenshots/        README interface previews
├── deploy/                  OpenResty and VPS deployment scripts
├── docker-compose.yml       Local and production container setup
├── Dockerfile
└── openapi.yaml             API contract
```

## License

This project is licensed under the [MIT License](LICENSE).
