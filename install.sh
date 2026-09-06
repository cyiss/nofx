#!/bin/sh
# Build and install this local, reviewed checkout. Do not pipe a remote script to bash.
# Usage: sh install.sh [path-to-existing-checkout]
set -eu
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "$0")" && pwd)
INSTALL_DIR="${1:-$SCRIPT_DIR}"
cd -- "$INSTALL_DIR"
for file in go.mod docker-compose.yml docker/Dockerfile.backend; do
    if [ ! -f "$file" ]; then
        echo "Missing $file. Clone and review the full repository before running this installer." >&2
        exit 1
    fi
done
command -v openssl >/dev/null || { echo 'OpenSSL is required.' >&2; exit 1; }
command -v docker >/dev/null || { echo 'Docker is required.' >&2; exit 1; }
docker info >/dev/null
if docker compose version >/dev/null 2>&1; then
    compose() { docker compose "$@"; }
elif command -v docker-compose >/dev/null; then
    compose() { docker-compose "$@"; }
else
    echo 'Docker Compose is required.' >&2
    exit 1
fi

# Never follow a symlink or overwrite existing encryption keys.
if [ -L .env ]; then
    echo 'Refusing a symlinked .env file.' >&2
    exit 1
fi
if [ -f .env ]; then
    chmod 600 .env
else
    JWT_SECRET=$(openssl rand -base64 32)
    DATA_ENCRYPTION_KEY=$(openssl rand -base64 32)
    RSA_PEM=$(openssl genrsa 2048 2>/dev/null)
    RSA_PRIVATE_KEY=$(printf '%s\n' "$RSA_PEM" | awk '{printf "%s\\n", $0}')
    cat > .env <<EOF
NOFX_BACKEND_PORT=8080
NOFX_FRONTEND_PORT=3000
TZ=Asia/Shanghai
JWT_SECRET=${JWT_SECRET}
DATA_ENCRYPTION_KEY=${DATA_ENCRYPTION_KEY}
RSA_PRIVATE_KEY=${RSA_PRIVATE_KEY}
EXPERIENCE_IMPROVEMENT=false
EOF
    chmod 600 .env
fi
mkdir -p data
compose -f docker-compose.yml config --quiet
compose -f docker-compose.yml build
compose -f docker-compose.yml up -d --no-build
printf 'Installed from %s\nWeb: http://127.0.0.1:3000\nAPI: http://127.0.0.1:8080\n' "$PWD"
printf 'Remote access requires an HTTPS reverse proxy. Updates require reviewing and rebuilding the local checkout.\n'
