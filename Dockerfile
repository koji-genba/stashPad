# ---- stage 1: frontend ビルド ----
FROM node:20-slim AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- stage 2: backend ビルド(dist を embed)----
# go.mod の go directive(modernc.org/sqlite が Go 1.25 を要求)と合わせること
FROM golang:1.25 AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/frontend/dist/ ./internal/web/dist/
RUN CGO_ENABLED=0 go build -o /stashpad ./cmd/stashpad

# ---- stage 3: 実行イメージ ----
# nonroot タグ: uid/gid 65532 で実行(非 root)。/data はこの uid が書けるようにすること。
FROM gcr.io/distroless/static:nonroot
COPY --from=backend /stashpad /stashpad
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD ["/stashpad", "-healthcheck"]
ENTRYPOINT ["/stashpad"]
