#############################
# Stage 1: build the bot
#############################
FROM golang:1.22 AS builder

ENV GOTOOLCHAIN=go1.25.4

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/bot ./cmd/bot

#############################
# Stage 2: final runtime
#############################
FROM alpine:3.19

RUN apk add --no-cache ca-certificates busybox-extras

ENV CHAT_WS_ADDR=:8080 \
  STATIC_PORT=4173 \
  STATIC_DIR=/srv/web \
  BUSYBOX_BIN=/bin/busybox-extras

WORKDIR /app

COPY --from=builder /app/bin/bot /app/bot
COPY docker-entrypoint.sh /usr/local/bin/entrypoint
RUN chmod +x /usr/local/bin/entrypoint /app/bot

# Static site (pre-built) is optional; copy if present
COPY web/build ${STATIC_DIR}

EXPOSE 8080 4173

ENTRYPOINT ["sh", "/usr/local/bin/entrypoint"]
