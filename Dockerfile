# Multi-stage build of the single static binary into an empty image.
FROM node:24-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/bookman ./cmd/bookman \
 && mkdir -p /out/tmp && chmod 1777 /out/tmp

FROM scratch
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=go /out/tmp /tmp
COPY --from=go /out/bookman /bookman
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/bookman"]
