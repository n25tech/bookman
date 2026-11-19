# Bookman

A self-hosted server for PDF and comic libraries: point it at folders of PDFs, CBZs and CBRs, browse them in a web UI (light and dark themes), read in the browser with a notes panel, and never lose your place. See [SPEC.md](SPEC.md) for the full design.

- **Single static binary** (`CGO_ENABLED=0`) with the Svelte UI embedded.
- **Formats:** PDF (rendered in the browser with PDF.js; covers and page counts with pdfium compiled to WebAssembly), CBZ and CBR (RAR4 and RAR5, solid or not). A `.cbr` that is really a zip, or the other way round, is detected from its contents.
- **Reading progress** is saved on every page change, every 2 s while scrolling, and with `sendBeacon` when the tab is hidden or closed. Saves that fail are queued in IndexedDB and retried. The last write wins by client time. If another device is further ahead, the reader offers to jump there.
- **Browse by folder:** a library opens on its folders exactly as they are on disk (e.g. `computer_science/` shows as "Computer Science"), with breadcrumbs, per-folder counts and covers. "All books" is the flat list; search covers the current folder and everything below it.
- **Notes** per page, optionally anchored to selected PDF text (shown as highlights). Markdown with preview, autosave, delete with undo, and export as Markdown.
- **The library is read-only.** Users, progress, notes and cover thumbnails live in **PostgreSQL**. Books are identified by a content hash, so moving or renaming a file keeps its progress and notes, and a deleted file is only marked missing.

## Development

You need Go 1.24+, Node 20+ and a PostgreSQL database.

| Command | What it does |
|---|---|
| `make build` | Builds the frontend (`web/dist`) and the static binary `bin/bookman` |
| `make test` | `go vet`, the Go tests and the frontend type check. Set `BOOKMAN_TEST_DATABASE_URL` to an empty database **that may be wiped** to include the API integration test |
| `make dev` | Runs the server with `bookman.dev.toml` (copy `bookman.example.toml`) and debug logging |
| `make dev-web` | Vite dev server on :5173 with hot reload, proxying `/api` to `BOOKMAN_API` (default `http://localhost:8080`) |
| `make test-e2e` | Playwright tests against a running server (below) |
| `make image` | Builds the container image from the `Dockerfile` (`FROM scratch`); CI does this for you |

The real-file format tests run when you point them at files: `BOOKMAN_TEST_CBR=x.cbr BOOKMAN_TEST_PDF=y.pdf BOOKMAN_TEST_CBZ=z.cbz go test ./internal/formats`.

### End-to-end test

The Playwright tests close a tab mid-book and check that the position (page and offset) comes back. They also check the "other device is further ahead" prompt, and the notes flow (select text, add a note, see the highlight after a reload, export, undo delete). They need a running server whose library contains `web/e2e/fixtures/test-book.pdf` (30 pages; recreate it with `python3 web/e2e/fixtures/make-test-pdf.py out.pdf 30`), plus a user:

```bash
cd web && npx playwright install chromium   # once
BOOKMAN_E2E_URL=http://localhost:8080 BOOKMAN_E2E_USER=admin BOOKMAN_E2E_PASSWORD=... make test-e2e
```

### CI

[.github/workflows/ci.yml](.github/workflows/ci.yml) runs on GitHub-hosted runners, which are free for public repositories:

- **Every push, and pull requests from forks:** `make build`, then `make test` with a PostgreSQL service container (so the API integration test runs). Then the Playwright tests run against a fresh server holding the fixture book.
- **Merges to `master`:** after the tests pass, it builds the `Dockerfile` and pushes `ghcr.io/n25tech/bookman:<12-char sha>` and `:prod` to GitHub Container Registry. It logs in with the workflow's own `GITHUB_TOKEN`, so no secret needs setting up.

No self-hosted runner is involved, so code from a fork never runs on the home cluster, and the cluster only ever pulls the public image.

## Configuration

`bookman -config /path/to/bookman.toml` (or `BOOKMAN_CONFIG`). Every key has an environment override; see [bookman.example.toml](bookman.example.toml).

| Key | Env | Default | |
|---|---|---|---|
| `listen` | `BOOKMAN_LISTEN` | `:8080` | |
| `database_url` | `BOOKMAN_DATABASE_URL` | empty | Empty uses `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, `PGPASSWORD` |
| `libraries` | `BOOKMAN_LIBRARIES` (`Name=/path;Name2=/path2`) | none | `{ name, path, exclude = [...] }`. Nested libraries are fine: a book belongs to the deepest library containing it |
| `rescan_interval` | `BOOKMAN_RESCAN_INTERVAL` | `6h` | Full rescan as a safety net next to the watcher |
| `watch` | `BOOKMAN_WATCH` | `true` | inotify watching of the library folders |
| `thumb_workers` | `BOOKMAN_THUMB_WORKERS` | CPUs/2 | Cover workers, and pdfium instances |
| `page_cache_mb` | `BOOKMAN_PAGE_CACHE_MB` | `128` | In-memory cache of extracted comic pages |

On first start with an empty database the UI asks you to create the admin account. Admins add more users and can start a scan from the Admin page.

## Deployment

### Docker

```bash
make image
docker run -d -p 8080:8080 -v /media/books:/media/books:ro -v $PWD/bookman.toml:/etc/bookman.toml:ro \
  -e PGHOST=db -e PGDATABASE=bookman -e PGUSER=bookman -e PGPASSWORD=... \
  registry.home/bookman/bookman -config /etc/bookman.toml
```

### systemd

```ini
# /etc/systemd/system/bookman.service
[Unit]
Description=Bookman
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/bookman -config /etc/bookman/bookman.toml
EnvironmentFile=/etc/bookman/env    # PGHOST=... PGPASSWORD=...
User=bookman
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### Kubernetes (the home cluster)

The manifests and the runbook live in the infrastructure repo: [n25tech/infrastructure `k8s/apps/bookman`](https://github.com/n25tech/infrastructure/tree/master/k8s/apps/bookman). There is one pod pinned to **a12**, with `/data/shared/ebooks` mounted read-only at `/library`. The configuration is a ConfigMap generated from `base/bookman.toml`, the data is in the PostgreSQL database `bookman`, and the image is `ghcr.io/n25tech/bookman:prod`. The UI is at https://bookman.home and https://bookman.k8s.master.home.

**Deploying a new build:** merge to `master` and wait for CI (`gh run list`). Then run `kubectl -n bookman rollout restart deploy/bookman`: the pod pulls `:prod` when it starts. Config changes, rollbacks and troubleshooting are in that Readme.
