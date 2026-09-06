#!/bin/sh
set -eu
repo=$(cd -- "$(dirname -- "$0")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
mkdir -p "$fixture/bin" "$fixture/checkout/docker" "$fixture/checkout/web"
cp "$repo/install.sh" "$fixture/checkout/install.sh"
cp "$repo/docker-compose.yml" "$fixture/checkout/docker-compose.yml"
cp "$repo/go.mod" "$fixture/checkout/go.mod"
cp "$repo/docker/Dockerfile.backend" "$fixture/checkout/docker/Dockerfile.backend"
cat > "$fixture/bin/docker" <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> "$INSTALL_TEST_LOG"
exit 0
STUB
cat > "$fixture/bin/curl" <<'STUB'
#!/bin/sh
printf '%s\n' "CURL $*" >> "$INSTALL_TEST_LOG"
exit 0
STUB
cat > "$fixture/bin/openssl" <<'STUB'
#!/bin/sh
if [ "$1" = rand ]; then
    printf 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n'
else
    [ "${INSTALL_TEST_RSA_FAIL:-0}" != 1 ] || exit 1
    printf '%s\n' '-----BEGIN PRIVATE KEY-----' 'SYNTHETIC-TEST-ONLY' '-----END PRIVATE KEY-----'
fi
STUB
chmod +x "$fixture/bin/"*
export PATH="$fixture/bin:$PATH" INSTALL_TEST_LOG="$fixture/calls"
umask 000
sh "$fixture/checkout/install.sh" "$fixture/checkout" > "$fixture/output" 2>&1
mode=$(stat -c %a "$fixture/checkout/.env")
[ "$mode" = 600 ] || { echo "FAIL: newly generated .env mode $mode, expected 600"; exit 1; }
! grep -q 'raw.githubusercontent.com' "$fixture/calls" || { echo 'FAIL: installer downloaded upstream configuration'; exit 1; }
grep -q 'build' "$fixture/calls" || { echo 'FAIL: installer did not build local source'; exit 1; }
cp "$fixture/checkout/.env" "$fixture/original-env"
chmod 666 "$fixture/checkout/.env"
sh "$fixture/checkout/install.sh" "$fixture/checkout" > "$fixture/output" 2>&1
[ "$(stat -c %a "$fixture/checkout/.env")" = 600 ] || { echo 'FAIL: existing .env not restricted'; exit 1; }
cmp "$fixture/original-env" "$fixture/checkout/.env"
rm "$fixture/checkout/.env"
ln -s "$fixture/original-env" "$fixture/checkout/.env"
if sh "$fixture/checkout/install.sh" "$fixture/checkout" > "$fixture/output" 2>&1; then
    echo 'FAIL: installer accepted symlinked secrets'; exit 1
fi
rm "$fixture/checkout/.env"
if INSTALL_TEST_RSA_FAIL=1 sh "$fixture/checkout/install.sh" "$fixture/checkout" > "$fixture/output" 2>&1; then
    echo 'FAIL: installer ignored RSA generation failure'; exit 1
fi
[ ! -e "$fixture/checkout/.env" ] || { echo 'FAIL: failed generation wrote .env'; exit 1; }
echo 'PASS: RSA failure, new/existing secret permissions, preserved keys, symlink refusal and local source build; Docker/network/OpenSSL mocked.'
