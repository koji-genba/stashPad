# ---- stage 1: frontend ビルド ----
FROM node:20-slim AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- stage 2: backend ビルド(dist を embed)----
FROM golang:1.24 AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/frontend/dist/ ./internal/web/dist/
RUN CGO_ENABLED=0 go build -o /stashpad ./cmd/stashpad

# ---- stage 3: 実行イメージ ----
FROM gcr.io/distroless/static
COPY --from=backend /stashpad /stashpad
EXPOSE 8080
ENTRYPOINT ["/stashpad"]
