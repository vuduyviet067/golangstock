# GoStock: one image, one address. The Go server answers the API and serves
# the built frontend from STATIC_DIR (DEP-02, DEP-03).
#
# Base images are pinned by tag and digest; each was published at least 14
# days before it was chosen (see README, "Deploy (Render + Neon)").

# 1) Frontend: npm ci + vite build. VITE_ALLOW_REGISTRATION is deliberately
#    not set, so the login screen has no registration (DEP-02).
FROM node:22.23.4-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# 2) Backend: static Go binary (embeds tzdata, CR-1). GOTOOLCHAIN=local makes
#    a go.mod version bump fail here instead of downloading another toolchain.
FROM golang:1.27.3-bookworm@sha256:dc0963f32053e4bc451d24d87e3892d42d01021b61f093e98eb95c0352bb84b0 AS backend
ENV GOTOOLCHAIN=local
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gostock .

# 3) Runtime: distroless static, no shell, non-root user.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=backend /out/gostock /app/gostock
COPY --from=frontend /src/frontend/dist /app/web
ENV STATIC_DIR=/app/web
USER nonroot
ENTRYPOINT ["/app/gostock"]
