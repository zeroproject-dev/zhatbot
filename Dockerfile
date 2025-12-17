#############################
# Stage 0: build the web (SvelteKit)
#############################
FROM oven/bun:1 AS web_builder

WORKDIR /web

ARG VITE_CHAT_WS_URL
ARG VITE_API_BASE_URL
ENV VITE_CHAT_WS_URL=${VITE_CHAT_WS_URL}
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

COPY web/bun.lock web/package.json ./
RUN bun install --frozen-lockfile

COPY web/ ./
RUN bun run build


#############################
# Stage 1: build the bot (Go + SQLite, multi-arch SAFE)
#############################
FROM golang:1.25-bookworm AS builder

# ENV GOTOOLCHAIN=go1.25.4
ENV CGO_ENABLED=1

WORKDIR /app

# Toolchain CGO + sqlite (glibc, NO musl)
RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates \
  gcc \
  g++ \
  libc6-dev \
  pkg-config \
  libsqlite3-dev \
  && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -o /app/bin/bot ./cmd/bot

#############################
# Stage 2: final runtime (NGINX + Debian)
#############################
FROM nginx:1.27-bookworm

# Quitar config default
RUN rm /etc/nginx/conf.d/default.conf

# Config SPA + API + WS
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Bot + frontend
COPY --from=builder /app/bin/bot /app/bot
COPY --from=web_builder /web/build /usr/share/nginx/html

# Entrypoint
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh /app/bot

EXPOSE 80 8080
ENTRYPOINT ["/docker-entrypoint.sh"]
