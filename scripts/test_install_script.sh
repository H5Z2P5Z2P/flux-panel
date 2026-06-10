#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d /tmp/flux-install-test.XXXXXX)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

die() {
  printf '[install-test] ERROR: %s\n' "$*" >&2
  exit 1
}

assert_file() {
  local path="$1"
  [[ -f "$path" ]] || die "missing file: $path"
}

assert_not_exists() {
  local path="$1"
  [[ ! -e "$path" ]] || die "expected path to be removed: $path"
}

assert_contains() {
  local path="$1"
  local needle="$2"
  grep -Fq "$needle" "$path" || die "expected $path to contain: $needle"
}

mkdir -p "$TMP_DIR/bin" "$TMP_DIR/install" "$TMP_DIR/systemd"

cat > "$TMP_DIR/bin/curl" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
out=""
while (($#)); do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    -L|-s|-S|--fail)
      shift
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$out" ]]; then
  exit 0
fi
cat > "$out" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-V" ]]; then
  echo "fake-gost 1.0"
  exit 0
fi
sleep 3600
EOF
chmod +x "$out"
SH
chmod +x "$TMP_DIR/bin/curl"

cat > "$TMP_DIR/bin/systemctl" <<SH
#!/usr/bin/env bash
echo "\$*" >> "$TMP_DIR/systemctl.log"
case "\${1:-}" in
  list-units)
    if [[ -f "$TMP_DIR/systemd/gost_flux_test.service" ]]; then
      echo "gost_flux_test.service loaded"
    fi
    ;;
  is-active)
    if [[ "\${2:-}" == "--quiet" ]]; then
      exit 0
    fi
    echo "active"
    ;;
esac
exit 0
SH
chmod +x "$TMP_DIR/bin/systemctl"

PATH="$TMP_DIR/bin:$PATH" \
INSTALL_DIR="$TMP_DIR/install" \
SYSTEMD_DIR="$TMP_DIR/systemd" \
SYSTEMCTL_BIN="$TMP_DIR/bin/systemctl" \
SERVICE_NAME="gost_flux_test" \
SKIP_TCPKILL_INSTALL=1 \
SKIP_DELETE_SELF=1 \
START_SERVICE=1 \
DOWNLOAD_URL="https://example.invalid/fake-gost" \
bash "$ROOT_DIR/install.sh" -a "http://panel.example:6365" -s "secret-123" >/tmp/flux-install-test-install.log

assert_file "$TMP_DIR/install/gost_flux"
assert_file "$TMP_DIR/install/config.json"
assert_file "$TMP_DIR/install/gost.json"
assert_file "$TMP_DIR/systemd/gost_flux_test.service"
assert_contains "$TMP_DIR/install/config.json" '"addr": "http://panel.example:6365"'
assert_contains "$TMP_DIR/install/config.json" '"secret": "secret-123"'
assert_contains "$TMP_DIR/systemd/gost_flux_test.service" "WorkingDirectory=$TMP_DIR/install"
assert_contains "$TMP_DIR/systemd/gost_flux_test.service" "ExecStart=$TMP_DIR/install/gost_flux"
assert_contains "$TMP_DIR/systemctl.log" "enable gost_flux_test"
assert_contains "$TMP_DIR/systemctl.log" "start gost_flux_test"

printf '2\n' | env \
  PATH="$TMP_DIR/bin:$PATH" \
  INSTALL_DIR="$TMP_DIR/install" \
  SYSTEMD_DIR="$TMP_DIR/systemd" \
  SYSTEMCTL_BIN="$TMP_DIR/bin/systemctl" \
  SERVICE_NAME="gost_flux_test" \
  SKIP_TCPKILL_INSTALL=1 \
  SKIP_DELETE_SELF=1 \
  START_SERVICE=0 \
  DOWNLOAD_URL="https://example.invalid/fake-gost-v2" \
  bash "$ROOT_DIR/install.sh" >/tmp/flux-install-test-update.log

assert_file "$TMP_DIR/install/gost_flux"
assert_not_exists "$TMP_DIR/install/gost_flux.new"
assert_contains "$TMP_DIR/systemctl.log" "stop gost_flux_test"

printf '3\n' | env \
  PATH="$TMP_DIR/bin:$PATH" \
  INSTALL_DIR="$TMP_DIR/install" \
  SYSTEMD_DIR="$TMP_DIR/systemd" \
  SYSTEMCTL_BIN="$TMP_DIR/bin/systemctl" \
  SERVICE_NAME="gost_flux_test" \
  SKIP_TCPKILL_INSTALL=1 \
  SKIP_DELETE_SELF=1 \
  AUTO_CONFIRM=1 \
  bash "$ROOT_DIR/install.sh" >/tmp/flux-install-test-uninstall.log

assert_not_exists "$TMP_DIR/install"
assert_not_exists "$TMP_DIR/systemd/gost_flux_test.service"
assert_contains "$TMP_DIR/systemctl.log" "disable gost_flux_test"
assert_contains "$TMP_DIR/systemctl.log" "daemon-reload"

printf '4\n' | env \
  PATH="$TMP_DIR/bin:$PATH" \
  INSTALL_DIR="$TMP_DIR/install" \
  SYSTEMD_DIR="$TMP_DIR/systemd" \
  SYSTEMCTL_BIN="$TMP_DIR/bin/systemctl" \
  SERVICE_NAME="gost_flux_test" \
  SKIP_TCPKILL_INSTALL=1 \
  SKIP_DELETE_SELF=1 \
  bash "$ROOT_DIR/install.sh" >/tmp/flux-install-test-exit.log

assert_contains "/tmp/flux-install-test-exit.log" "退出脚本"

printf '[install-test] PASS\n'
