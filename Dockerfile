# --- build stage ---
FROM golang:1.26.9-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static, stripped binary.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- run stage ---
FROM alpine:3.21
# apk upgrade pulls patched base packages (e.g. openssl) newer than the image.
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates wget \
    && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/api /app/api
# Drop root: run as an unprivileged user.
USER 10001
EXPOSE 8080
# Liveness probe for orchestrators that honor HEALTHCHECK.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/api"]
