#!/bin/sh
set -e

cleanup() {
  if [ -n "$HTTPD_PID" ] && kill -0 "$HTTPD_PID" 2>/dev/null; then
    kill "$HTTPD_PID"
  fi

  if [ -n "$BOT_PID" ] && kill -0 "$BOT_PID" 2>/dev/null; then
    kill "$BOT_PID"
  fi
}

trap cleanup INT TERM

/app/bot &
BOT_PID=$!

if [ -d "$STATIC_DIR" ] && [ "$(ls -A "$STATIC_DIR")" ]; then
  HTTPD_BIN=${BUSYBOX_BIN:-"/bin/busybox"}
  if [ ! -x "$HTTPD_BIN" ]; then
    HTTPD_BIN=$(command -v busybox || true)
  fi

  if [ -x "$HTTPD_BIN" ]; then
    "$HTTPD_BIN" httpd -f -p "${STATIC_PORT}" -h "$STATIC_DIR" &
    HTTPD_PID=$!
    echo "Static site served on http://0.0.0.0:${STATIC_PORT}"
  else
    echo "BusyBox httpd binary not found; skipping static server"
  fi
else
  echo "Static directory $STATIC_DIR is empty or missing; skipping static server"
fi

wait $BOT_PID
