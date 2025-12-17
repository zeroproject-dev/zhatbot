#!/bin/sh
set -e

# Start backend
/app/bot &

# Runtime config para SPA
cat >/usr/share/nginx/html/config.js <<EOF
window.__CONFIG__ = {
  WS_URL: "${VITE_CHAT_WS_URL}",
  API_BASE_URL: "${VITE_API_BASE_URL}"
};
EOF

# Start nginx
exec nginx -g "daemon off;"
