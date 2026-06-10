#!/usr/bin/env bash
set -Eeuo pipefail

: "${PANEL_HOST:?PANEL_HOST is required}"
ENTRY_HOST="${ENTRY_HOST:-$PANEL_HOST}"
: "${EXIT_A_HOST:?EXIT_A_HOST is required}"
: "${EXIT_B_HOST:?EXIT_B_HOST is required}"
PANEL_PORT="${PANEL_PORT:-6365}"
BASE_DIR="${BASE_DIR:-/tmp/flux-panel-e2e}"
BACKEND_BIN="${BACKEND_BIN:-/tmp/flux-panel-e2e-backend}"
AGENT_BIN="${AGENT_BIN:-/tmp/flux-panel-e2e-agent}"
JWT_SECRET="${JWT_SECRET:-e2e-secret}"

ENTRY_FORWARD_PORT="${ENTRY_FORWARD_PORT:-37010}"
TUNNEL_FORWARD_PORT="${TUNNEL_FORWARD_PORT:-37011}"
TEMP_FORWARD_PORT="${TEMP_FORWARD_PORT:-37012}"
ORPHAN_FORWARD_PORT="${ORPHAN_FORWARD_PORT:-37013}"
ENTRY_PORT_RANGES="${ENTRY_PORT_RANGES:-37010-37080}"
ENTRY_A_PORT_RANGES="${ENTRY_A_PORT_RANGES:-37010-37080}"
EXIT_A_PORT_RANGES="${EXIT_A_PORT_RANGES:-37110-37180}"
EXIT_B_PORT_RANGES="${EXIT_B_PORT_RANGES:-37210-37280}"
TARGET_A_PORT="${TARGET_A_PORT:-39081}"
TARGET_LB_B_PORT="${TARGET_LB_B_PORT:-39083}"
TARGET_B_PORT="${TARGET_B_PORT:-39082}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TOKEN=""
NODE_ENTRY_SECRET=""
NODE_ENTRY_A_SECRET=""
NODE_EXIT_A_SECRET=""
NODE_EXIT_B_SECRET=""
NODE_ENTRY_ID=""
NODE_ENTRY_A_ID=""
NODE_EXIT_A_ID=""
NODE_EXIT_B_ID=""
TUNNEL_TYPE1_ID=""
TUNNEL_TYPE2_ID=""
FORWARD_LB_ID=""
FORWARD_TUNNEL_ID=""
TEMP_FORWARD_ID=""
ORPHAN_FORWARD_ID=""
RUNTIME_USER_ID=""
RUNTIME_USER_TUNNEL_ID=""
RUNTIME_FORWARD_ID=""
RUNTIME_MANUAL_FORWARD_ID=""
SPEED_LIMIT_ID=""
TUNNEL_UDP_BEFORE=""
TUNNEL_UDP_AFTER=""
TUNNEL_ENTRY_AFTER=""
TUNNEL_UDP_ENTRY_AFTER=""

log() {
  printf '[e2e] %s\n' "$*"
}

die() {
  printf '[e2e] ERROR: %s\n' "$*" >&2
  exit 1
}

ssh_host() {
  local host="$1"
  shift
  local attempt
  for attempt in 1 2 3 4 5; do
    if ssh -o BatchMode=yes -o ConnectTimeout=10 "root@${host}" "$@"; then
      return 0
    fi
    sleep "$attempt"
  done
  return 255
}

scp_to() {
  local src="$1"
  local host="$2"
  local dst="$3"
  local attempt
  for attempt in 1 2 3 4 5; do
    if scp -q -o BatchMode=yes -o ConnectTimeout=10 "$src" "root@${host}:${dst}"; then
      return 0
    fi
    sleep "$attempt"
  done
  return 255
}

json_get() {
  local expr="$1"
  python3 -c "import json,sys; data=json.load(sys.stdin); print(${expr})"
}

api() {
  local path="$1"
  local payload="${2:-{}}"
  local attempt
  for attempt in 1 2 3 4 5; do
    if curl -sS --fail --connect-timeout 5 --max-time 20 -X POST "http://${PANEL_HOST}:${PANEL_PORT}${path}" \
      -H "Authorization: Bearer ${TOKEN}" \
      -H 'Content-Type: application/json' \
      -d "$payload"; then
      return 0
    fi
    sleep "$attempt"
  done
  return 1
}

api_login() {
  local out=""
  local attempt
  for attempt in 1 2 3 4 5; do
    if out="$(curl -sS --fail --connect-timeout 5 --max-time 20 -X POST "http://${PANEL_HOST}:${PANEL_PORT}/api/v1/user/login" \
      -H 'Content-Type: application/json' \
      -d '{"username":"admin_user","password":"admin_user"}')"; then
      TOKEN="$(printf '%s' "$out" | json_get 'data["data"]["token"]')"
      break
    fi
    sleep "$attempt"
  done
  test -n "$TOKEN" || die "login returned empty token"
}

api_expect_ok() {
  local path="$1"
  local payload="$2"
  local out
  out="$(api "$path" "$payload")"
  python3 - "$out" <<'PY'
import json, sys
data=json.loads(sys.argv[1])
if data.get("code") != 0:
    raise SystemExit(f"api failed: {data}")
PY
  printf '%s\n' "$out"
}

api_assert_diag() {
  local path="$1"
  local payload="$2"
  local expected_port="$3"
  local expected_target="${4:-}"
  local out
  out="$(api_expect_ok "$path" "$payload")"
  python3 - "$out" "$expected_port" "$expected_target" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
expected_port = int(sys.argv[2])
expected_target = sys.argv[3]
results = data.get("data", {}).get("results") or []
if not results:
    raise SystemExit(f"diagnose returned no results: {data}")
if expected_port and not any(int(r.get("targetPort", -1)) == expected_port for r in results):
    raise SystemExit(f"diagnose missing targetPort {expected_port}: {results}")
if expected_target and not any(str(r.get("targetIp", "")) == expected_target for r in results):
    raise SystemExit(f"diagnose missing targetIp {expected_target}: {results}")
failed = [r for r in results if not r.get("success")]
if failed:
    raise SystemExit(f"diagnose contains failed results: {failed}")
PY
  printf '%s\n' "$out"
}

api_assert_diag_targets() {
  local path="$1"
  local payload="$2"
  shift 2
  local out
  out="$(api_expect_ok "$path" "$payload")"
  python3 - "$out" "$@" <<'PY'
import json, sys
data = json.loads(sys.argv[1])
expected = []
for item in sys.argv[2:]:
    host, port = item.rsplit(":", 1)
    expected.append((host, int(port)))
results = data.get("data", {}).get("results") or []
seen = {(str(r.get("targetIp", "")), int(r.get("targetPort", -1))) for r in results}
missing = [f"{host}:{port}" for host, port in expected if (host, port) not in seen]
if missing:
    raise SystemExit(f"diagnose missing targets {missing}: {results}")
failed = [r for r in results if not r.get("success")]
if failed:
    raise SystemExit(f"diagnose contains failed results: {failed}")
PY
  printf '%s\n' "$out"
}

db_scalar() {
  local sql="$1"
  ssh_host "$PANEL_HOST" "python3 - <<PY
import sqlite3
con=sqlite3.connect('${BASE_DIR}/panel/data/flux.db')
row=con.execute(\"\"\"${sql}\"\"\").fetchone()
assert row is not None, 'query returned no rows'
print(row[0])
PY"
}

db_exec() {
  local sql="$1"
  ssh_host "$PANEL_HOST" "python3 - <<PY
import sqlite3
con=sqlite3.connect('${BASE_DIR}/panel/data/flux.db')
con.executescript(\"\"\"${sql}\"\"\")
con.commit()
PY"
}

create_node() {
  local name="$1"
  local ip="$2"
  local server_ip="$3"
  local ranges="$4"
  api_expect_ok "/api/v1/node/create" "{\"name\":\"${name}\",\"ip\":\"${ip}\",\"serverIp\":\"${server_ip}\",\"portRanges\":\"${ranges}\",\"http\":0,\"tls\":0,\"socks\":0}" >/dev/null
  ssh_host "$PANEL_HOST" "python3 - <<PY
import sqlite3
con=sqlite3.connect('${BASE_DIR}/panel/data/flux.db')
row=con.execute('select id, secret from node where name=? order by id desc limit 1', ('${name}',)).fetchone()
assert row, 'node not found'
print(f'{row[0]} {row[1]}')
PY"
}

cleanup_host() {
  local host="$1"
  ssh_host "$host" "set +e
find ${BASE_DIR} -mindepth 2 -maxdepth 2 -name '*.pid' -type f 2>/dev/null | while IFS= read -r f; do
  pid=\$(cat \"\$f\")
  case \"\$pid\" in (*[!0-9]*|\"\") :;; (*) if kill -0 \"\$pid\" 2>/dev/null; then kill \"\$pid\"; fi;; esac
done
sleep 1
find ${BASE_DIR} -mindepth 2 -maxdepth 2 -name '*.pid' -type f 2>/dev/null | while IFS= read -r f; do
  pid=\$(cat \"\$f\")
  case \"\$pid\" in (*[!0-9]*|\"\") :;; (*) if kill -0 \"\$pid\" 2>/dev/null; then kill -9 \"\$pid\"; fi;; esac
done
rm -rf ${BASE_DIR}
rm -f ${BACKEND_BIN} ${AGENT_BIN}" || true
}

cleanup() {
  set +e
  cleanup_host "$PANEL_HOST"
  cleanup_host "$ENTRY_HOST"
  cleanup_host "$EXIT_A_HOST"
  cleanup_host "$EXIT_B_HOST"
}

trap cleanup EXIT

backup_and_clean_known_residue() {
  local host="$1"
  ssh_host "$host" "set -e
stamp=\$(date +%Y%m%d%H%M%S)
backup_dir=/root/flux-panel-residue-backup-\$stamp
has_residue=0
if [ -d /etc/gost_flux ] || [ -f /etc/systemd/system/gost_flux.service ]; then
  mkdir -p \"\$backup_dir\"
  has_residue=1
fi
if [ -d /etc/gost_flux ]; then
  tar -C /etc -czf \"\$backup_dir/etc_gost_flux.tar.gz\" gost_flux
fi
if [ -f /etc/systemd/system/gost_flux.service ]; then
  cp -a /etc/systemd/system/gost_flux.service \"\$backup_dir/gost_flux.service\"
fi
active=\$(systemctl is-active gost_flux 2>/dev/null || true)
if [ \"\$active\" = active ]; then
  echo \"gost_flux active; backed up known paths to \$backup_dir and left service untouched\" >&2
  exit 42
fi
if [ -d /etc/gost_flux ]; then
  mv /etc/gost_flux \"\$backup_dir/etc_gost_flux.cleaned\"
fi
if [ -f /etc/systemd/system/gost_flux.service ]; then
  systemctl disable gost_flux >/dev/null 2>&1 || true
  mv /etc/systemd/system/gost_flux.service \"\$backup_dir/gost_flux.service.cleaned\"
  systemctl daemon-reload >/dev/null 2>&1 || true
fi
if [ \"\$has_residue\" = 1 ]; then
  echo \"backup=\$backup_dir\"
else
  echo no-known-residue
fi"
}

assert_port_free() {
  local host="$1"
  local regex="$2"
  local found
  found="$(ssh_host "$host" "ss -ltnup | grep -E '${regex}' || true")"
  if [ -n "$found" ]; then
    printf '%s\n' "$found" >&2
    die "ports already in use on ${host}: ${regex}"
  fi
}

wait_for_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    if curl -sS --connect-timeout 1 --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "timeout waiting for ${url}"
}

wait_for_node_count() {
  local expected="$1"
  for _ in $(seq 1 60); do
    local count
    count="$(db_scalar 'select count(*) from node where status=1')"
    if [ "$count" = "$expected" ]; then
      return 0
    fi
    sleep 1
  done
  die "timeout waiting for ${expected} online nodes"
}

start_target() {
  local host="$1"
  local dir="$2"
  local port="$3"
  local body="$4"
  ssh_host "$host" "mkdir -p '${BASE_DIR}/${dir}' && cat > '${BASE_DIR}/${dir}/server.py' <<'PY'
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import sys
BODY = sys.argv[1].encode()
PORT = int(sys.argv[2])
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(BODY)
    def log_message(self, *args):
        pass
ThreadingHTTPServer(('0.0.0.0', PORT), H).serve_forever()
PY
mkdir -p '${BASE_DIR}/${dir}' && cd '${BASE_DIR}/${dir}' || exit 1
nohup python3 server.py '${body}' '${port}' </dev/null > target.log 2>&1 &
echo \$! > target.pid"
}

start_udp_target() {
  local host="$1"
  local dir="$2"
  local port="$3"
  local body="$4"
  ssh_host "$host" "mkdir -p '${BASE_DIR}/${dir}' && cat > '${BASE_DIR}/${dir}/udp_server.py' <<'PY'
import socket
import sys
BODY = sys.argv[1].encode()
PORT = int(sys.argv[2])
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(('0.0.0.0', PORT))
while True:
    data, addr = sock.recvfrom(65535)
    sock.sendto(BODY + b':' + data, addr)
PY
mkdir -p '${BASE_DIR}/${dir}' && cd '${BASE_DIR}/${dir}' || exit 1
nohup python3 udp_server.py '${body}' '${port}' </dev/null > udp-target.log 2>&1 &
echo \$! > udp-target.pid"
}

start_agent() {
  local host="$1"
  local dir="$2"
  local secret="$3"
  ssh_host "$host" "mkdir -p '${BASE_DIR}/${dir}' && cp '${AGENT_BIN}' '${BASE_DIR}/${dir}/gost-agent-e2e' && cat > '${BASE_DIR}/${dir}/config.json' <<JSON
{\"addr\":\"http://${PANEL_HOST}:${PANEL_PORT}\",\"secret\":\"${secret}\",\"http\":0,\"tls\":0,\"socks\":0}
JSON
cat > '${BASE_DIR}/${dir}/gost.json' <<JSON
{}
JSON
mkdir -p '${BASE_DIR}/${dir}' && cd '${BASE_DIR}/${dir}' || exit 1
nohup ./gost-agent-e2e </dev/null > agent.log 2>&1 &
echo \$! > agent.pid"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    die "expected output to contain ${needle}, got: ${haystack}"
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" == *"$needle"* ]]; then
    die "expected output not to contain ${needle}, got: ${haystack}"
  fi
}

agent_json_value() {
  local host="$1"
  local dir="$2"
  local py="$3"
  ssh_host "$host" "python3 - <<PY
import json, pathlib
p=pathlib.Path('${BASE_DIR}/${dir}/gost.json')
d=json.load(open(p)) if p.exists() else {}
${py}
PY"
}

agent_config_value() {
  local host="$1"
  local dir="$2"
  local py="$3"
  ssh_host "$host" "python3 - <<PY
import json, pathlib
p=pathlib.Path('${BASE_DIR}/${dir}/config.json')
d=json.load(open(p)) if p.exists() else {}
${py}
PY"
}

service_names() {
  local host="$1"
  local dir="$2"
  agent_json_value "$host" "$dir" "print([s.get('name') for s in (d.get('services') or [])])"
}

chain_names() {
  local host="$1"
  local dir="$2"
  agent_json_value "$host" "$dir" "print([c.get('name') for c in (d.get('chains') or [])])"
}

wait_for_service_present() {
  local host="$1"
  local dir="$2"
  local service="$3"
  local names=""
  for _ in $(seq 1 30); do
    names="$(service_names "$host" "$dir")"
    if [[ "$names" == *"$service"* ]]; then
      return 0
    fi
    sleep 1
  done
  die "service ${service} not present in ${host}/${dir}: ${names}"
}

wait_for_chain_present() {
  local host="$1"
  local dir="$2"
  local chain="$3"
  local names=""
  for _ in $(seq 1 30); do
    names="$(chain_names "$host" "$dir")"
    if [[ "$names" == *"$chain"* ]]; then
      return 0
    fi
    sleep 1
  done
  die "chain ${chain} not present in ${host}/${dir}: ${names}"
}

remote_http_get() {
  local host="$1"
  local port="$2"
  ssh_host "$host" "curl -sS --connect-timeout 3 --max-time 8 http://127.0.0.1:${port}/"
}

wait_for_remote_body() {
  local host="$1"
  local port="$2"
  local expected="$3"
  local out=""
  for _ in $(seq 1 30); do
    out="$(remote_http_get "$host" "$port" 2>/dev/null || true)"
    if [[ "$out" == *"$expected"* ]]; then
      printf '%s\n' "$out"
      return 0
    fi
    sleep 1
  done
  die "timeout waiting for ${host}:${port} to return ${expected}, last output: ${out}"
}

assert_remote_http_unavailable() {
  local host="$1"
  local port="$2"
  ssh_host "$host" true >/dev/null || die "ssh unavailable while checking ${host}:${port}"
  for _ in $(seq 1 12); do
    if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "root@${host}" "curl -sS --connect-timeout 1 --max-time 2 http://127.0.0.1:${port}/ >/dev/null" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "expected ${host}:${port} to be unavailable"
}

remote_udp_get() {
  local host="$1"
  local port="$2"
  local payload="$3"
  ssh_host "$host" "python3 - <<PY
import socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.settimeout(5)
sock.sendto(b'${payload}', ('127.0.0.1', ${port}))
data, _ = sock.recvfrom(65535)
print(data.decode())
PY"
}

wait_for_remote_udp_body() {
  local host="$1"
  local port="$2"
  local payload="$3"
  local expected="$4"
  local out=""
  for _ in $(seq 1 30); do
    out="$(remote_udp_get "$host" "$port" "$payload" 2>/dev/null || true)"
    if [[ "$out" == *"$expected"* ]]; then
      printf '%s\n' "$out"
      return 0
    fi
    sleep 1
  done
  die "timeout waiting for UDP ${host}:${port} to return ${expected}, last output: ${out}"
}

assert_remote_udp_unavailable() {
  local host="$1"
  local port="$2"
  ssh_host "$host" true >/dev/null || die "ssh unavailable while checking UDP ${host}:${port}"
  for _ in $(seq 1 8); do
    if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "root@${host}" "python3 - <<PY
import socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.settimeout(2)
sock.sendto(b'closed', ('127.0.0.1', ${port}))
sock.recvfrom(65535)
PY" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  die "expected UDP ${host}:${port} to be unavailable"
}

wait_for_service_absent() {
  local host="$1"
  local dir="$2"
  local service="$3"
  local names=""
  for _ in $(seq 1 30); do
    names="$(service_names "$host" "$dir")"
    if [[ "$names" != *"$service"* ]]; then
      return 0
    fi
    sleep 1
  done
  die "service ${service} still present in ${host}/${dir}: ${names}"
}

wait_for_chain_absent() {
  local host="$1"
  local dir="$2"
  local chain="$3"
  local names=""
  for _ in $(seq 1 30); do
    names="$(chain_names "$host" "$dir")"
    if [[ "$names" != *"$chain"* ]]; then
      return 0
    fi
    sleep 1
  done
  die "chain ${chain} still present in ${host}/${dir}: ${names}"
}

wait_for_limiter_limit() {
  local host="$1"
  local dir="$2"
  local limiter="$3"
  local expected="$4"
  local limits=""
  for _ in $(seq 1 30); do
    limits="$(agent_json_value "$host" "$dir" "items=[l for l in (d.get('limiters') or []) if l.get('name')=='${limiter}']; print(items[0].get('limits') if items else '')")"
    if [[ "$limits" == *"$expected"* ]]; then
      return 0
    fi
    sleep 1
  done
  die "limiter ${limiter} did not contain ${expected}, got: ${limits}"
}

wait_for_limiter_absent() {
  local host="$1"
  local dir="$2"
  local limiter="$3"
  local names=""
  for _ in $(seq 1 30); do
    names="$(agent_json_value "$host" "$dir" "print([l.get('name') for l in (d.get('limiters') or [])])")"
    if [[ "$names" != *"'${limiter}'"* && "$names" != *"\"${limiter}\""* ]]; then
      return 0
    fi
    sleep 1
  done
  die "limiter ${limiter} still present: ${names}"
}

wait_for_agent_config_value() {
  local host="$1"
  local dir="$2"
  local key="$3"
  local expected="$4"
  local value=""
  for _ in $(seq 1 30); do
    value="$(agent_config_value "$host" "$dir" "print(d.get('${key}', ''))")"
    if [ "$value" = "$expected" ]; then
      return 0
    fi
    sleep 1
  done
  die "config ${key} expected ${expected}, got ${value}"
}

post_agent_config_report() {
  local host="$1"
  local dir="$2"
  local secret="$3"
  ssh_host "$host" "curl -sS --connect-timeout 5 --max-time 20 -X POST 'http://${PANEL_HOST}:${PANEL_PORT}/flow/config?secret=${secret}' \
    -H 'Content-Type: application/json' --data-binary '@${BASE_DIR}/${dir}/gost.json' >/dev/null"
}

log "checking SSH, tools and port availability"
for host in "$PANEL_HOST" "$ENTRY_HOST" "$EXIT_A_HOST" "$EXIT_B_HOST"; do
  ssh_host "$host" "command -v python3 >/dev/null && command -v ss >/dev/null && command -v curl >/dev/null" >/dev/null
done

log "backing up and isolating known residue on exit-b ${EXIT_B_HOST}"
backup_and_clean_known_residue "$EXIT_B_HOST" || die "exit-b has active gost_flux residue; stop or isolate it before running this test"

assert_port_free "$PANEL_HOST" ":(${PANEL_PORT})\\b"
assert_port_free "$ENTRY_HOST" ":(${ENTRY_FORWARD_PORT}|${TUNNEL_FORWARD_PORT}|${TEMP_FORWARD_PORT}|${ORPHAN_FORWARD_PORT})\\b"
assert_port_free "$EXIT_A_HOST" ":(${ENTRY_FORWARD_PORT}|${TUNNEL_FORWARD_PORT}|${TEMP_FORWARD_PORT}|${ORPHAN_FORWARD_PORT}|${TARGET_A_PORT}|${TARGET_B_PORT}|37110|37210)\\b"
assert_port_free "$EXIT_B_HOST" ":(${TARGET_LB_B_PORT}|${TARGET_B_PORT}|37110|37210)\\b"

log "building backend and agent"
(cd "$ROOT_DIR/go-backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BACKEND_BIN" ./)
(cd "$ROOT_DIR/go-gost" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o "$AGENT_BIN" ./)

log "cleaning old temporary state"
cleanup_host "$PANEL_HOST"
cleanup_host "$ENTRY_HOST"
cleanup_host "$EXIT_A_HOST"
cleanup_host "$EXIT_B_HOST"

log "deploying binaries"
scp_to "$BACKEND_BIN" "$PANEL_HOST" "$BACKEND_BIN"
scp_to "$AGENT_BIN" "$ENTRY_HOST" "$AGENT_BIN"
scp_to "$AGENT_BIN" "$EXIT_A_HOST" "$AGENT_BIN"
scp_to "$AGENT_BIN" "$EXIT_B_HOST" "$AGENT_BIN"

log "starting temporary panel on ${PANEL_HOST}:${PANEL_PORT}"
ssh_host "$PANEL_HOST" "mkdir -p '${BASE_DIR}/panel/data' && cd '${BASE_DIR}/panel' || exit 1
nohup env SERVER_PORT='${PANEL_PORT}' DB_TYPE=sqlite DB_NAME='${BASE_DIR}/panel/data/flux.db' JWT_SECRET='${JWT_SECRET}' '${BACKEND_BIN}' </dev/null > panel.log 2>&1 &
echo \$! > panel.pid"
wait_for_http "http://${PANEL_HOST}:${PANEL_PORT}/flow/test"
api_login

log "creating panel nodes"
read -r NODE_ENTRY_ID NODE_ENTRY_SECRET < <(create_node "e2e-entry-172" "$ENTRY_HOST" "$ENTRY_HOST" "$ENTRY_PORT_RANGES")
read -r NODE_ENTRY_A_ID NODE_ENTRY_A_SECRET < <(create_node "e2e-entry-a-161" "$EXIT_A_HOST" "$EXIT_A_HOST" "$ENTRY_A_PORT_RANGES")
read -r NODE_EXIT_A_ID NODE_EXIT_A_SECRET < <(create_node "e2e-exit-a-161" "$EXIT_A_HOST" "$EXIT_A_HOST" "$EXIT_A_PORT_RANGES")
read -r NODE_EXIT_B_ID NODE_EXIT_B_SECRET < <(create_node "e2e-exit-b-104" "$EXIT_B_HOST" "$EXIT_B_HOST" "$EXIT_B_PORT_RANGES")

log "starting target services and agents"
start_target "$EXIT_A_HOST" "target-lb-a" "$TARGET_A_PORT" "LB_TARGET_A_161"
start_target "$EXIT_A_HOST" "target-tunnel-a" "$TARGET_B_PORT" "TUNNEL_TARGET_EXIT_A_161"
start_target "$EXIT_B_HOST" "target-lb-b" "$TARGET_LB_B_PORT" "LB_TARGET_B_104"
start_target "$EXIT_B_HOST" "target-tunnel-b" "$TARGET_B_PORT" "TUNNEL_TARGET_EXIT_B_104"
start_udp_target "$EXIT_A_HOST" "target-udp-lb-a" "$TARGET_A_PORT" "UDP_LB_TARGET_A_161"
start_udp_target "$EXIT_A_HOST" "target-udp-tunnel-a" "$TARGET_B_PORT" "UDP_TUNNEL_TARGET_EXIT_A_161"
start_udp_target "$EXIT_B_HOST" "target-udp-lb-b" "$TARGET_LB_B_PORT" "UDP_LB_TARGET_B_104"
start_udp_target "$EXIT_B_HOST" "target-udp-tunnel-b" "$TARGET_B_PORT" "UDP_TUNNEL_TARGET_EXIT_B_104"
start_agent "$ENTRY_HOST" "agent-entry" "$NODE_ENTRY_SECRET"
start_agent "$EXIT_A_HOST" "agent-entry-a" "$NODE_ENTRY_A_SECRET"
start_agent "$EXIT_A_HOST" "agent-exit-a" "$NODE_EXIT_A_SECRET"
start_agent "$EXIT_B_HOST" "agent-exit-b" "$NODE_EXIT_B_SECRET"
wait_for_node_count 4

log "creating type1 load-balanced forward across exit-a and exit-b"
api_expect_ok "/api/v1/tunnel/create" "{\"name\":\"e2e-type1-lb\",\"inNodeId\":${NODE_ENTRY_ID},\"type\":1,\"flow\":2,\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
TUNNEL_TYPE1_ID="$(db_scalar "select id from tunnel where name='e2e-type1-lb'")"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-lb-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}, ${EXIT_B_HOST}:${TARGET_LB_B_PORT}\",\"inPort\":${ENTRY_FORWARD_PORT},\"strategy\":\"ROUND-ROBIN\"}" >/dev/null
FORWARD_LB_ID="$(db_scalar "select id from forward where name='e2e-lb-forward'")"
sleep 2

lb_result="$(ssh_host "$ENTRY_HOST" "for i in \$(seq 1 10); do curl -sS --connect-timeout 3 --max-time 8 http://127.0.0.1:${ENTRY_FORWARD_PORT}/; echo; done")"
assert_contains "$lb_result" "LB_TARGET_A_161"
assert_contains "$lb_result" "LB_TARGET_B_104"
lb_udp_result="$(wait_for_remote_udp_body "$ENTRY_HOST" "$ENTRY_FORWARD_PORT" "lb-udp" "UDP_")"
if [[ "$lb_udp_result" != *"UDP_LB_TARGET_A_161"* && "$lb_udp_result" != *"UDP_LB_TARGET_B_104"* ]]; then
  die "load-balanced UDP forward returned unexpected body: ${lb_udp_result}"
fi
selector_strategy="$(agent_json_value "$ENTRY_HOST" "agent-entry" "svc=[s for s in d.get('services',[]) if s.get('name')=='${FORWARD_LB_ID}_1_0_tcp'][0]; print(svc['forwarder']['selector']['strategy'])")"
test "$selector_strategy" = "round" || die "expected selector strategy round, got ${selector_strategy}"

log "diagnosing type1 tunnel and forward"
api_assert_diag_targets "/api/v1/tunnel/diagnose" "{\"tunnelId\":${TUNNEL_TYPE1_ID}}" "${EXIT_A_HOST}:${TARGET_A_PORT}" "${EXIT_B_HOST}:${TARGET_LB_B_PORT}" >/dev/null
api_assert_diag_targets "/api/v1/forward/diagnose" "{\"forwardId\":${FORWARD_LB_ID}}" "${EXIT_A_HOST}:${TARGET_A_PORT}" "${EXIT_B_HOST}:${TARGET_LB_B_PORT}" >/dev/null

log "pausing and resuming load-balanced forward"
api_expect_ok "/api/v1/forward/pause" "{\"id\":${FORWARD_LB_ID}}" >/dev/null
sleep 2
forward_status="$(db_scalar "select status from forward where id=${FORWARD_LB_ID}")"
test "$forward_status" = "0" || die "expected paused forward status 0, got ${forward_status}"
paused_flag="$(agent_json_value "$ENTRY_HOST" "agent-entry" "svc=[s for s in d.get('services',[]) if s.get('name')=='${FORWARD_LB_ID}_1_0_tcp'][0]; print(svc.get('metadata',{}).get('paused'))")"
test "$paused_flag" = "True" || die "expected paused metadata, got ${paused_flag}"
assert_remote_http_unavailable "$ENTRY_HOST" "$ENTRY_FORWARD_PORT"
assert_remote_udp_unavailable "$ENTRY_HOST" "$ENTRY_FORWARD_PORT"
api_expect_ok "/api/v1/forward/resume" "{\"id\":${FORWARD_LB_ID}}" >/dev/null
sleep 2
forward_status="$(db_scalar "select status from forward where id=${FORWARD_LB_ID}")"
test "$forward_status" = "1" || die "expected resumed forward status 1, got ${forward_status}"
wait_for_remote_body "$ENTRY_HOST" "$ENTRY_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
lb_udp_resume_result="$(wait_for_remote_udp_body "$ENTRY_HOST" "$ENTRY_FORWARD_PORT" "lb-udp-resume" "UDP_")"
if [[ "$lb_udp_resume_result" != *"UDP_LB_TARGET_A_161"* && "$lb_udp_resume_result" != *"UDP_LB_TARGET_B_104"* ]]; then
  die "resumed load-balanced UDP forward returned unexpected body: ${lb_udp_resume_result}"
fi

log "updating load-balanced forward strategy"
api_expect_ok "/api/v1/forward/update" "{\"id\":${FORWARD_LB_ID},\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-lb-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}, ${EXIT_B_HOST}:${TARGET_LB_B_PORT}\",\"inPort\":${ENTRY_FORWARD_PORT},\"strategy\":\"random\"}" >/dev/null
sleep 2
selector_strategy="$(agent_json_value "$ENTRY_HOST" "agent-entry" "svc=[s for s in d.get('services',[]) if s.get('name')=='${FORWARD_LB_ID}_1_0_tcp'][0]; print(svc['forwarder']['selector']['strategy'])")"
test "$selector_strategy" = "random" || die "expected selector strategy random, got ${selector_strategy}"
lb_update_result="$(remote_http_get "$ENTRY_HOST" "$ENTRY_FORWARD_PORT")"
if [[ "$lb_update_result" != *"LB_TARGET_A_161"* && "$lb_update_result" != *"LB_TARGET_B_104"* ]]; then
  die "updated load-balanced forward returned unexpected body: ${lb_update_result}"
fi

log "testing user and user-tunnel disable/reenable runtime sync"
RUNTIME_EXP_TIME="$(ssh_host "$PANEL_HOST" "python3 - <<'PY'
import time
print(int((time.time()+86400)*1000))
PY")"
api_expect_ok "/api/v1/user/create" "{\"user\":\"e2e-runtime-user\",\"pwd\":\"e2e-runtime-user\",\"status\":1,\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
RUNTIME_USER_ID="$(db_scalar "select id from user where user='e2e-runtime-user'")"
api_expect_ok "/api/v1/tunnel/user/assign" "{\"userId\":${RUNTIME_USER_ID},\"tunnelId\":${TUNNEL_TYPE1_ID},\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
RUNTIME_USER_TUNNEL_ID="$(db_scalar "select id from user_tunnel where user_id=${RUNTIME_USER_ID} and tunnel_id=${TUNNEL_TYPE1_ID}")"
RUNTIME_FORWARD_PORT=$((ENTRY_FORWARD_PORT + 20))
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-runtime-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}\",\"inPort\":${RUNTIME_FORWARD_PORT},\"strategy\":\"fifo\",\"userId\":${RUNTIME_USER_ID}}" >/dev/null
RUNTIME_FORWARD_ID="$(db_scalar "select id from forward where name='e2e-runtime-forward'")"
runtime_service="${RUNTIME_FORWARD_ID}_${RUNTIME_USER_ID}_${RUNTIME_USER_TUNNEL_ID}_tcp"
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
RUNTIME_MANUAL_FORWARD_PORT=$((ENTRY_FORWARD_PORT + 21))
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-runtime-manual-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}\",\"inPort\":${RUNTIME_MANUAL_FORWARD_PORT},\"strategy\":\"fifo\",\"userId\":${RUNTIME_USER_ID}}" >/dev/null
RUNTIME_MANUAL_FORWARD_ID="$(db_scalar "select id from forward where name='e2e-runtime-manual-forward'")"
runtime_manual_service="${RUNTIME_MANUAL_FORWARD_ID}_${RUNTIME_USER_ID}_${RUNTIME_USER_TUNNEL_ID}_tcp"
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_MANUAL_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
api_expect_ok "/api/v1/forward/pause" "{\"id\":${RUNTIME_MANUAL_FORWARD_ID}}" >/dev/null
sleep 2
manual_status="$(db_scalar "select status from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
manual_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
test "$manual_status" = "0" || die "expected manually paused forward status 0, got ${manual_status}"
test "$manual_reason" = "0" || die "expected manually paused forward pause_reason 0, got ${manual_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_MANUAL_FORWARD_PORT"

api_expect_ok "/api/v1/tunnel/user/update" "{\"id\":${RUNTIME_USER_TUNNEL_ID},\"speedId\":0,\"status\":0}" >/dev/null
sleep 2
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "0" || die "expected user-tunnel-disabled forward status 0, got ${runtime_status}"
test "$runtime_reason" = "2" || die "expected user-tunnel-disabled forward pause_reason 2, got ${runtime_reason}"
runtime_paused="$(agent_json_value "$ENTRY_HOST" "agent-entry" "svc=[s for s in d.get('services',[]) if s.get('name')=='${runtime_service}'][0]; print(svc.get('metadata',{}).get('paused'))")"
test "$runtime_paused" = "True" || die "expected user-tunnel pause metadata, got ${runtime_paused}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"
api_expect_ok "/api/v1/tunnel/user/update" "{\"id\":${RUNTIME_USER_TUNNEL_ID},\"speedId\":0,\"status\":1}" >/dev/null
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "1" || die "expected user-tunnel-reenabled forward status 1, got ${runtime_status}"
test "$runtime_reason" = "0" || die "expected user-tunnel-reenabled forward pause_reason 0, got ${runtime_reason}"
runtime_paused="$(agent_json_value "$ENTRY_HOST" "agent-entry" "svc=[s for s in d.get('services',[]) if s.get('name')=='${runtime_service}'][0]; print(svc.get('metadata',{}).get('paused'))")"
test "$runtime_paused" = "None" || die "expected user-tunnel resume to clear pause metadata, got ${runtime_paused}"
manual_status="$(db_scalar "select status from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
manual_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
test "$manual_status" = "0" || die "expected manual forward to remain paused after user-tunnel reenable, got ${manual_status}"
test "$manual_reason" = "0" || die "expected manual forward pause_reason to remain 0 after user-tunnel reenable, got ${manual_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_MANUAL_FORWARD_PORT"

api_expect_ok "/api/v1/user/update" "{\"id\":${RUNTIME_USER_ID},\"user\":\"e2e-runtime-user\",\"status\":0,\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
sleep 2
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "0" || die "expected user-disabled forward status 0, got ${runtime_status}"
test "$runtime_reason" = "1" || die "expected user-disabled forward pause_reason 1, got ${runtime_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"
api_expect_ok "/api/v1/user/update" "{\"id\":${RUNTIME_USER_ID},\"user\":\"e2e-runtime-user\",\"status\":1,\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "1" || die "expected user-reenabled forward status 1, got ${runtime_status}"
test "$runtime_reason" = "0" || die "expected user-reenabled forward pause_reason 0, got ${runtime_reason}"
manual_status="$(db_scalar "select status from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
manual_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_MANUAL_FORWARD_ID}")"
test "$manual_status" = "0" || die "expected manual forward to remain paused after user reenable, got ${manual_status}"
test "$manual_reason" = "0" || die "expected manual forward pause_reason to remain 0 after user reenable, got ${manual_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_MANUAL_FORWARD_PORT"

api_expect_ok "/api/v1/user/update" "{\"id\":${RUNTIME_USER_ID},\"user\":\"e2e-runtime-user\",\"status\":0,\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
api_expect_ok "/api/v1/tunnel/user/update" "{\"id\":${RUNTIME_USER_TUNNEL_ID},\"speedId\":0,\"status\":0}" >/dev/null
sleep 2
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "0" || die "expected stacked-block forward status 0, got ${runtime_status}"
test "$runtime_reason" = "3" || die "expected stacked-block forward pause_reason 3, got ${runtime_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"
api_expect_ok "/api/v1/user/update" "{\"id\":${RUNTIME_USER_ID},\"user\":\"e2e-runtime-user\",\"status\":1,\"flow\":99999,\"num\":10,\"expTime\":${RUNTIME_EXP_TIME},\"flowResetTime\":1}" >/dev/null
sleep 2
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "0" || die "expected forward to stay paused while user tunnel remains disabled, got ${runtime_status}"
test "$runtime_reason" = "2" || die "expected stacked-block forward pause_reason 2 after user reenable, got ${runtime_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"
api_expect_ok "/api/v1/tunnel/user/update" "{\"id\":${RUNTIME_USER_TUNNEL_ID},\"speedId\":0,\"status\":1}" >/dev/null
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "1" || die "expected stacked-block forward to resume after both clears, got ${runtime_status}"
test "$runtime_reason" = "0" || die "expected stacked-block forward pause_reason 0 after both clears, got ${runtime_reason}"

db_exec "update user_tunnel set flow=1, in_flow=0, out_flow=0 where id=${RUNTIME_USER_TUNNEL_ID}; update user set flow=99999, in_flow=0, out_flow=0 where id=${RUNTIME_USER_ID};"
ssh_host "$ENTRY_HOST" "curl -sS --connect-timeout 5 --max-time 20 -X POST 'http://${PANEL_HOST}:${PANEL_PORT}/flow/upload?secret=${NODE_ENTRY_SECRET}' -H 'Content-Type: application/json' -d '{\"n\":\"${RUNTIME_FORWARD_ID}_${RUNTIME_USER_ID}_${RUNTIME_USER_TUNNEL_ID}\",\"d\":1073741824,\"v\":1}' >/dev/null"
sleep 2
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
test "$runtime_status" = "0" || die "expected flow upload to pause user-tunnel-flow-blocked forward, got ${runtime_status}"
test "$runtime_reason" = "2" || die "expected flow upload to set pause_reason 2, got ${runtime_reason}"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"
api_expect_ok "/api/v1/user/reset" "{\"id\":${RUNTIME_USER_ID},\"type\":1}" >/dev/null
wait_for_remote_body "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
runtime_status="$(db_scalar "select status from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_reason="$(db_scalar "select pause_reason from forward where id=${RUNTIME_FORWARD_ID}")"
runtime_ut_flow="$(db_scalar "select in_flow + out_flow from user_tunnel where id=${RUNTIME_USER_TUNNEL_ID}")"
test "$runtime_status" = "1" || die "expected user reset to resume user-tunnel-flow-blocked forward, got ${runtime_status}"
test "$runtime_reason" = "0" || die "expected user reset to clear user-tunnel pause_reason, got ${runtime_reason}"
test "$runtime_ut_flow" = "0" || die "expected user reset to clear user_tunnel flow, got ${runtime_ut_flow}"

api_expect_ok "/api/v1/forward/delete" "{\"id\":${RUNTIME_MANUAL_FORWARD_ID}}" >/dev/null
wait_for_service_absent "$ENTRY_HOST" "agent-entry" "$runtime_manual_service"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_MANUAL_FORWARD_PORT"
api_expect_ok "/api/v1/forward/delete" "{\"id\":${RUNTIME_FORWARD_ID}}" >/dev/null
wait_for_service_absent "$ENTRY_HOST" "agent-entry" "$runtime_service"
assert_remote_http_unavailable "$ENTRY_HOST" "$RUNTIME_FORWARD_PORT"

log "creating and deleting a temporary port forward"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-delete-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}\",\"inPort\":${TEMP_FORWARD_PORT},\"strategy\":\"fifo\"}" >/dev/null
TEMP_FORWARD_ID="$(db_scalar "select id from forward where name='e2e-delete-forward'")"
wait_for_remote_body "$ENTRY_HOST" "$TEMP_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
api_expect_ok "/api/v1/forward/delete" "{\"id\":${TEMP_FORWARD_ID}}" >/dev/null
wait_for_service_absent "$ENTRY_HOST" "agent-entry" "${TEMP_FORWARD_ID}_1_0_tcp"
assert_remote_http_unavailable "$ENTRY_HOST" "$TEMP_FORWARD_PORT"

log "creating real stale forward residue and cleaning it through config report"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"e2e-orphan-forward\",\"remoteAddr\":\"${EXIT_A_HOST}:${TARGET_A_PORT}\",\"inPort\":${ORPHAN_FORWARD_PORT},\"strategy\":\"fifo\"}" >/dev/null
ORPHAN_FORWARD_ID="$(db_scalar "select id from forward where name='e2e-orphan-forward'")"
wait_for_remote_body "$ENTRY_HOST" "$ORPHAN_FORWARD_PORT" "LB_TARGET_A_161" >/dev/null
db_exec "delete from forward where id=${ORPHAN_FORWARD_ID};"
post_agent_config_report "$ENTRY_HOST" "agent-entry" "$NODE_ENTRY_SECRET"
wait_for_service_absent "$ENTRY_HOST" "agent-entry" "${ORPHAN_FORWARD_ID}_1_0_tcp"
assert_remote_http_unavailable "$ENTRY_HOST" "$ORPHAN_FORWARD_PORT"

log "creating type2 tunnel and forward through exit-a"
api_expect_ok "/api/v1/tunnel/create" "{\"name\":\"e2e-type2\",\"inNodeId\":${NODE_ENTRY_ID},\"outNodeId\":${NODE_EXIT_A_ID},\"type\":2,\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
TUNNEL_TYPE2_ID="$(db_scalar "select id from tunnel where name='e2e-type2'")"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE2_ID},\"name\":\"e2e-type2-forward\",\"remoteAddr\":\"127.0.0.1:${TARGET_B_PORT}\",\"inPort\":${TUNNEL_FORWARD_PORT},\"strategy\":\"fifo\"}" >/dev/null
FORWARD_TUNNEL_ID="$(db_scalar "select id from forward where name='e2e-type2-forward'")"
sleep 2
tunnel_before="$(wait_for_remote_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "TUNNEL_TARGET_EXIT_A_161")"
TUNNEL_UDP_BEFORE="$(wait_for_remote_udp_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "tunnel-udp-before" "UDP_TUNNEL_TARGET_EXIT_A_161")"
relay_a_before="$(service_names "$EXIT_A_HOST" "agent-exit-a")"
assert_contains "$relay_a_before" "tunnel_${TUNNEL_TYPE2_ID}_relay"

log "diagnosing type2 tunnel and forward"
api_assert_diag "/api/v1/tunnel/diagnose" "{\"tunnelId\":${TUNNEL_TYPE2_ID}}" "$TARGET_B_PORT" "127.0.0.1" >/dev/null
api_assert_diag "/api/v1/forward/diagnose" "{\"forwardId\":${FORWARD_TUNNEL_ID}}" "$TARGET_B_PORT" "127.0.0.1" >/dev/null

log "creating, updating and deleting tunnel speed limit"
api_expect_ok "/api/v1/speed-limit/create" "{\"name\":\"e2e-limit\",\"speed\":16,\"tunnelId\":${TUNNEL_TYPE2_ID},\"tunnelName\":\"e2e-type2\"}" >/dev/null
SPEED_LIMIT_ID="$(db_scalar "select id from speed_limit where name='e2e-limit'")"
wait_for_limiter_limit "$ENTRY_HOST" "agent-entry" "$SPEED_LIMIT_ID" "2.0MB"
api_expect_ok "/api/v1/speed-limit/update" "{\"id\":${SPEED_LIMIT_ID},\"name\":\"e2e-limit\",\"speed\":24,\"tunnelId\":${TUNNEL_TYPE2_ID},\"tunnelName\":\"e2e-type2\"}" >/dev/null
wait_for_limiter_limit "$ENTRY_HOST" "agent-entry" "$SPEED_LIMIT_ID" "3.0MB"
api_expect_ok "/api/v1/speed-limit/delete" "{\"id\":${SPEED_LIMIT_ID}}" >/dev/null
wait_for_limiter_absent "$ENTRY_HOST" "agent-entry" "$SPEED_LIMIT_ID"

log "switching tunnel exit from exit-a to exit-b"
api_expect_ok "/api/v1/tunnel/update" "{\"id\":${TUNNEL_TYPE2_ID},\"name\":\"e2e-type2\",\"inNodeId\":${NODE_ENTRY_ID},\"outNodeId\":${NODE_EXIT_B_ID},\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
tunnel_after="$(wait_for_remote_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "TUNNEL_TARGET_EXIT_B_104")"
TUNNEL_UDP_AFTER="$(wait_for_remote_udp_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "tunnel-udp-after" "UDP_TUNNEL_TARGET_EXIT_B_104")"
chain_after="$(agent_json_value "$ENTRY_HOST" "agent-entry" "chains=d.get('chains') or []; print(chains)")"
assert_contains "$chain_after" "${EXIT_B_HOST}:37210"
relay_a_after="$(service_names "$EXIT_A_HOST" "agent-exit-a")"
assert_not_contains "$relay_a_after" "tunnel_${TUNNEL_TYPE2_ID}_relay"
relay_b_after="$(service_names "$EXIT_B_HOST" "agent-exit-b")"
assert_contains "$relay_b_after" "tunnel_${TUNNEL_TYPE2_ID}_relay"

sockets_old="$(ssh_host "$EXIT_A_HOST" "ss -ltnup | grep -E ':37110\\b' || true")"
assert_not_contains "$sockets_old" "gost-agent-e2e"
sockets_new="$(ssh_host "$EXIT_B_HOST" "ss -ltnup | grep -E ':37210\\b' || true")"
assert_contains "$sockets_new" "gost-agent-e2e"

log "switching tunnel entry from 172 to 161 and verifying forward relocation"
api_expect_ok "/api/v1/tunnel/update" "{\"id\":${TUNNEL_TYPE2_ID},\"name\":\"e2e-type2\",\"inNodeId\":${NODE_ENTRY_A_ID},\"outNodeId\":${NODE_EXIT_B_ID},\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
wait_for_service_absent "$ENTRY_HOST" "agent-entry" "${FORWARD_TUNNEL_ID}_1_0_tcp"
wait_for_chain_absent "$ENTRY_HOST" "agent-entry" "tunnel_${TUNNEL_TYPE2_ID}_chains"
wait_for_service_present "$EXIT_A_HOST" "agent-entry-a" "${FORWARD_TUNNEL_ID}_1_0_tcp"
wait_for_chain_present "$EXIT_A_HOST" "agent-entry-a" "tunnel_${TUNNEL_TYPE2_ID}_chains"
assert_remote_http_unavailable "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT"
assert_remote_udp_unavailable "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT"
TUNNEL_ENTRY_AFTER="$(wait_for_remote_body "$EXIT_A_HOST" "$TUNNEL_FORWARD_PORT" "TUNNEL_TARGET_EXIT_B_104")"
TUNNEL_UDP_ENTRY_AFTER="$(wait_for_remote_udp_body "$EXIT_A_HOST" "$TUNNEL_FORWARD_PORT" "tunnel-udp-entry-after" "UDP_TUNNEL_TARGET_EXIT_B_104")"

log "updating entry node protocol flags over http/ws-compatible agent connection"
api_expect_ok "/api/v1/node/update" "{\"id\":${NODE_ENTRY_ID},\"name\":\"e2e-entry-172\",\"ip\":\"${ENTRY_HOST}\",\"serverIp\":\"${ENTRY_HOST}\",\"portRanges\":\"${ENTRY_PORT_RANGES}\",\"http\":1,\"tls\":0,\"socks\":0}" >/dev/null
wait_for_agent_config_value "$ENTRY_HOST" "agent-entry" "http" "1"
api_expect_ok "/api/v1/node/update" "{\"id\":${NODE_ENTRY_ID},\"name\":\"e2e-entry-172\",\"ip\":\"${ENTRY_HOST}\",\"serverIp\":\"${ENTRY_HOST}\",\"portRanges\":\"${ENTRY_PORT_RANGES}\",\"http\":0,\"tls\":0,\"socks\":0}" >/dev/null
wait_for_agent_config_value "$ENTRY_HOST" "agent-entry" "http" "0"

log "final smoke"
printf '[e2e] load-balance responses:\n%s\n' "$lb_result"
printf '[e2e] load-balance UDP response:\n%s\n' "$lb_udp_result"
printf '[e2e] load-balance after update:\n%s\n' "$lb_update_result"
printf '[e2e] tunnel before switch:\n%s\n' "$tunnel_before"
printf '[e2e] tunnel UDP before switch:\n%s\n' "$TUNNEL_UDP_BEFORE"
printf '[e2e] tunnel after switch:\n%s\n' "$tunnel_after"
printf '[e2e] tunnel UDP after switch:\n%s\n' "$TUNNEL_UDP_AFTER"
printf '[e2e] tunnel after entry switch:\n%s\n' "$TUNNEL_ENTRY_AFTER"
printf '[e2e] tunnel UDP after entry switch:\n%s\n' "$TUNNEL_UDP_ENTRY_AFTER"
printf '[e2e] tunnel forward id: %s\n' "$FORWARD_TUNNEL_ID"
log "PASS"
