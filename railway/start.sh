#!/bin/sh
set -eu
umask 077
# Persist these values in Railway variables. Never rotate encryption keys at startup.
: "${JWT_SECRET:?Configure a persistent JWT_SECRET}"
: "${DATA_ENCRYPTION_KEY:?Configure a persistent DATA_ENCRYPTION_KEY}"
: "${RSA_PRIVATE_KEY:?Configure a persistent RSA_PRIVATE_KEY}"
PORT=${PORT:-8080}
case "$PORT" in ''|*[!0-9]*) echo 'PORT must be numeric' >&2; exit 1;; esac
[ "$PORT" -ge 1 ] && [ "$PORT" -le 65535 ] && [ "$PORT" -ne 8081 ] || exit 1
export PORT
cat > /etc/nginx/http.d/default.conf <<NGINX_EOF
server {
    listen $PORT;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    gzip on;
    gzip_types text/plain text/css application/json application/javascript;
    location / { try_files \$uri \$uri/ /index.html; }
    location /api/ {
        proxy_pass http://127.0.0.1:8081/api/;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_connect_timeout 30s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
    location = /health {
        proxy_pass http://127.0.0.1:8081/api/health;
        access_log off;
    }
}
NGINX_EOF
backend_pid=
nginx_pid=
cleanup() {
    trap - EXIT INT TERM
    [ -z "$nginx_pid" ] || kill "$nginx_pid" 2>/dev/null || true
    [ -z "$backend_pid" ] || kill "$backend_pid" 2>/dev/null || true
    wait || true
}
trap cleanup EXIT
trap 'exit 0' INT TERM
API_SERVER_HOST=127.0.0.1 API_SERVER_PORT=8081 /app/nofx &
backend_pid=$!
nginx -g 'daemon off;' &
nginx_pid=$!
while kill -0 "$backend_pid" 2>/dev/null && kill -0 "$nginx_pid" 2>/dev/null; do
    sleep 1
done
echo 'NOFX or nginx exited; stopping container.' >&2
exit 1
