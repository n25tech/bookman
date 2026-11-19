# Bookman

A self-hosted server for PDF and comic libraries: point it at folders of PDFs, CBZs and CBRs, browse them in a web UI (light and dark themes), read in the browser with a notes panel, and never lose your place. See [SPEC.md](SPEC.md) for the full design.

- **Single static binary** (`CGO_ENABLED=0`) with the Svelte UI embedded.
- **Formats:** PDF (rendered in the browser with PDF.js; covers and page counts with pdfium compiled to WebAssembly), CBZ and CBR (RAR4 and RAR5, solid or not). A `.cbr` that is really a zip, or the other way round, is detected from its contents.
- **Reading progress** is saved on every page change, every 2 s while scrolling, and with `sendBeacon` when the tab is hidden or closed. Saves that fail are queued in IndexedDB and retried. The last write wins by client time. If another device is further ahead, the reader offers to jump there.
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
| `make image` | Builds the container image from the `Dockerfile` (`FROM scratch`) |

The real-file format tests run when you point them at files: `BOOKMAN_TEST_CBR=x.cbr BOOKMAN_TEST_PDF=y.pdf BOOKMAN_TEST_CBZ=z.cbz go test ./internal/formats`.

### End-to-end test

The Playwright tests close a tab mid-book and check that the position (page and offset) comes back. They also check the "other device is further ahead" prompt, and the notes flow (select text, add a note, see the highlight after a reload, export, undo delete). They need a running server whose library contains `web/e2e/fixtures/test-book.pdf` (30 pages; recreate it with `python3 web/e2e/fixtures/make-test-pdf.py out.pdf 30`), plus a user:

```bash
cd web && npx playwright install chromium   # once
BOOKMAN_E2E_URL=http://localhost:8080 BOOKMAN_E2E_USER=admin BOOKMAN_E2E_PASSWORD=... make test-e2e
```

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

The manifests are in [deploy/k8s](deploy/k8s): one pod pinned to **a12**, which has the books at `/data/shared/ebooks`. That folder is mounted read-only at `/library`. The configuration is the `bookman-config` ConfigMap, generated from [deploy/k8s/bookman.toml](deploy/k8s/bookman.toml). The data is in the PostgreSQL database `bookman` on `postgres.dbs.svc` ([infrastructure: k8s/dbs/postgres](../infrastructure/k8s/dbs/postgres/Readme.md)). The UI is at `https://bookman.home` (and `https://bookman.k8s.master.home`) through the home-gateway.

1. **Database (done once, on a12):** the role and database `bookman`, the `postgres-bookman` Secret in the `bookman` namespace, and a copy of the login in `~/.creds/postgres/bookman`, as in postgres step 3 with `APP=bookman`. (A second database, `bookman_test`, owned by the same role, is for the integration test.)
2. **Image:** build and push `registry.home/bookman/bookman:<tag>` (`make push`, or `sudo nerdctl build` + `push` on a12), and set `newTag` in `deploy/k8s/kustomization.yaml`.
3. **TLS name:** `bookman.k8s.master.home` already matches the edge cert's `*.k8s.master.home`. For `bookman.home`, add `DNS:bookman.home` to the edge cert ([pki step 4](../infrastructure/pki/Readme.md)) and `sudo nerdctl restart edge-envoy` on a12.
4. **On every machine you browse from:** `echo '192.168.1.11 bookman.home bookman.k8s.master.home' | sudo tee -a /etc/hosts`
5. **Deploy:**

   ```bash
   kubectl apply -k deploy/k8s
   kubectl -n bookman rollout status deploy/bookman
   kubectl -n bookman logs -f deploy/bookman      # "scan finished ... added=N"
   ```

**Changing the config:** edit `deploy/k8s/bookman.toml` and `kubectl apply -k deploy/k8s`. The ConfigMap name carries a hash, so the pod restarts with the new file.

**Why PostgreSQL rather than RustFS:** progress saves are small, frequent upserts that need a transaction (last write wins by client time, with reading sessions next to them). That is a database job, and the cluster already runs one. Cover thumbnails (~30 KB each) go in the same database, so the pod needs no volume and nothing else to back up.

| Symptom | Fix |
|---|---|
| Pod `CreateContainerConfigError`: `secret "postgres-bookman" not found` | Step 1 |
| Log: `postgres not reachable yet` for over a minute | `kubectl -n dbs get pod`; compare the Secret with `~/.creds/postgres/bookman` |
| Log: `library root: ... no such file or directory` | The hostPath or the `path` in `bookman.toml` is wrong |
| Browser: `no alternative certificate subject name matches 'bookman.home'` | Step 3, or use `bookman.k8s.master.home` |
| A book shows "This file couldn't be read" | It's corrupt or encrypted; the error is on its detail page and in the Admin page's last error |
