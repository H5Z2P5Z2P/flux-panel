#!/usr/bin/env bash
set -Eeuo pipefail

: "${PANEL_HOST:?PANEL_HOST is required}"
ENTRY_HOST="${ENTRY_HOST:-$PANEL_HOST}"
: "${EXIT_HOST:?EXIT_HOST is required}"
PANEL_PORT="${PANEL_PORT:-6366}"
BASE_DIR="${BASE_DIR:-/tmp/flux-panel-e2e-dual}"
BACKEND_BIN="${BACKEND_BIN:-/tmp/flux-panel-e2e-dual-backend}"
AGENT_BIN="${AGENT_BIN:-/tmp/flux-panel-e2e-dual-agent}"
JWT_SECRET="${JWT_SECRET:-e2e-dual-secret}"

ENTRY_FORWARD_PORT="${ENTRY_FORWARD_PORT:-37310}"
TUNNEL_FORWARD_PORT="${TUNNEL_FORWARD_PORT:-37311}"
ENTRY_PORT_RANGES="${ENTRY_PORT_RANGES:-37310-37380}"
ENTRY_B_PORT_RANGES="${ENTRY_B_PORT_RANGES:-37310-37380}"
EXIT_A_PORT_RANGES="${EXIT_A_PORT_RANGES:-37410-37480}"
EXIT_B_PORT_RANGES="${EXIT_B_PORT_RANGES:-37510-37580}"
TARGET_A_PORT="${TARGET_A_PORT:-39381}"
TARGET_TUNNEL_PORT="${TARGET_TUNNEL_PORT:-39382}"
TARGET_B_PORT="${TARGET_B_PORT:-39383}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TOKEN=""
NODE_ENTRY_ID=""
NODE_ENTRY_SECRET=""
NODE_ENTRY_B_ID=""
NODE_ENTRY_B_SECRET=""
NODE_EXIT_A_ID=""
NODE_EXIT_A_SECRET=""
NODE_EXIT_B_ID=""
NODE_EXIT_B_SECRET=""
TUNNEL_TYPE1_ID=""
TUNNEL_TYPE2_ID=""
FORWARD_LB_ID=""
FORWARD_TUNNEL_ID=""

log() {
  printf '[dual-e2e] %s\n' "$*"
}

die() {
  printf '[dual-e2e] ERROR: %s\n' "$*" >&2
  exit 1
}

ssh_host() {
  local host="$1"
  shift
  local attempt
  for attempt in 1 2 3 4 5; do
    if ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=10 "root@${host}" "$@"; then
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
    if scp -q -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=10 "$src" "root@${host}:${dst}"; then
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
  cleanup_host "$EXIT_HOST"
}

trap cleanup EXIT

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
cd '${BASE_DIR}/${dir}' || exit 1
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
cd '${BASE_DIR}/${dir}' || exit 1
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
cd '${BASE_DIR}/${dir}' || exit 1
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
    if ! ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=5 "root@${host}" "curl -sS --connect-timeout 1 --max-time 2 http://127.0.0.1:${port}/ >/dev/null" >/dev/null 2>&1; then
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
    if ! ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=5 "root@${host}" "python3 - <<PY
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

log "checking SSH, tools and port availability"
for host in "$PANEL_HOST" "$ENTRY_HOST" "$EXIT_HOST"; do
  ssh_host "$host" "command -v python3 >/dev/null && command -v ss >/dev/null && command -v curl >/dev/null" >/dev/null
done

assert_port_free "$PANEL_HOST" ":(${PANEL_PORT})\\b"
assert_port_free "$ENTRY_HOST" ":(${ENTRY_FORWARD_PORT}|${TUNNEL_FORWARD_PORT})\\b"
assert_port_free "$EXIT_HOST" ":(${ENTRY_FORWARD_PORT}|${TUNNEL_FORWARD_PORT}|${TARGET_A_PORT}|${TARGET_TUNNEL_PORT}|${TARGET_B_PORT}|37410|37510)\\b"

log "building backend and agent"
(cd "$ROOT_DIR/go-backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BACKEND_BIN" ./)
(cd "$ROOT_DIR/go-gost" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o "$AGENT_BIN" ./)

log "cleaning old temporary state"
cleanup_host "$PANEL_HOST"
cleanup_host "$ENTRY_HOST"
cleanup_host "$EXIT_HOST"

log "deploying binaries"
scp_to "$BACKEND_BIN" "$PANEL_HOST" "$BACKEND_BIN"
scp_to "$AGENT_BIN" "$ENTRY_HOST" "$AGENT_BIN"
scp_to "$AGENT_BIN" "$EXIT_HOST" "$AGENT_BIN"

log "starting temporary panel on ${PANEL_HOST}:${PANEL_PORT}"
ssh_host "$PANEL_HOST" "mkdir -p '${BASE_DIR}/panel/data' && cd '${BASE_DIR}/panel' || exit 1
nohup env SERVER_PORT='${PANEL_PORT}' DB_TYPE=sqlite DB_NAME='${BASE_DIR}/panel/data/flux.db' JWT_SECRET='${JWT_SECRET}' '${BACKEND_BIN}' </dev/null > panel.log 2>&1 &
echo \$! > panel.pid"
wait_for_http "http://${PANEL_HOST}:${PANEL_PORT}/flow/test"
api_login

log "creating panel nodes"
read -r NODE_ENTRY_ID NODE_ENTRY_SECRET < <(create_node "dual-entry-172" "$ENTRY_HOST" "$ENTRY_HOST" "$ENTRY_PORT_RANGES")
read -r NODE_ENTRY_B_ID NODE_ENTRY_B_SECRET < <(create_node "dual-entry-209" "$EXIT_HOST" "$EXIT_HOST" "$ENTRY_B_PORT_RANGES")
read -r NODE_EXIT_A_ID NODE_EXIT_A_SECRET < <(create_node "dual-exit-a-209" "$EXIT_HOST" "$EXIT_HOST" "$EXIT_A_PORT_RANGES")
read -r NODE_EXIT_B_ID NODE_EXIT_B_SECRET < <(create_node "dual-exit-b-209" "$EXIT_HOST" "$EXIT_HOST" "$EXIT_B_PORT_RANGES")

log "starting target services and agents"
start_target "$EXIT_HOST" "target-lb-a" "$TARGET_A_PORT" "DUAL_LB_TARGET_A_209"
start_target "$EXIT_HOST" "target-tunnel" "$TARGET_TUNNEL_PORT" "DUAL_TUNNEL_TARGET_209"
start_target "$EXIT_HOST" "target-lb-b" "$TARGET_B_PORT" "DUAL_LB_TARGET_B_209"
start_udp_target "$EXIT_HOST" "target-udp-lb-a" "$TARGET_A_PORT" "DUAL_UDP_LB_TARGET_A_209"
start_udp_target "$EXIT_HOST" "target-udp-tunnel" "$TARGET_TUNNEL_PORT" "DUAL_UDP_TUNNEL_TARGET_209"
start_udp_target "$EXIT_HOST" "target-udp-lb-b" "$TARGET_B_PORT" "DUAL_UDP_LB_TARGET_B_209"
start_agent "$ENTRY_HOST" "agent-entry-172" "$NODE_ENTRY_SECRET"
start_agent "$EXIT_HOST" "agent-entry-209" "$NODE_ENTRY_B_SECRET"
start_agent "$EXIT_HOST" "agent-exit-a-209" "$NODE_EXIT_A_SECRET"
start_agent "$EXIT_HOST" "agent-exit-b-209" "$NODE_EXIT_B_SECRET"
wait_for_node_count 4

log "testing type1 load-balanced TCP and UDP forward"
api_expect_ok "/api/v1/tunnel/create" "{\"name\":\"dual-type1-lb\",\"inNodeId\":${NODE_ENTRY_ID},\"type\":1,\"flow\":2,\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
TUNNEL_TYPE1_ID="$(db_scalar "select id from tunnel where name='dual-type1-lb'")"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"dual-lb-forward\",\"remoteAddr\":\"${EXIT_HOST}:${TARGET_A_PORT}, ${EXIT_HOST}:${TARGET_B_PORT}\",\"inPort\":${ENTRY_FORWARD_PORT},\"strategy\":\"ROUND-ROBIN\"}" >/dev/null
FORWARD_LB_ID="$(db_scalar "select id from forward where name='dual-lb-forward'")"
sleep 2

lb_result="$(ssh_host "$ENTRY_HOST" "for i in \$(seq 1 8); do curl -sS --connect-timeout 3 --max-time 8 http://127.0.0.1:${ENTRY_FORWARD_PORT}/; echo; done")"
assert_contains "$lb_result" "DUAL_LB_TARGET_A_209"
assert_contains "$lb_result" "DUAL_LB_TARGET_B_209"
lb_udp_result="$(wait_for_remote_udp_body "$ENTRY_HOST" "$ENTRY_FORWARD_PORT" "dual-lb-udp" "DUAL_UDP_LB_TARGET_")"
api_assert_diag_targets "/api/v1/tunnel/diagnose" "{\"tunnelId\":${TUNNEL_TYPE1_ID}}" "${EXIT_HOST}:${TARGET_A_PORT}" "${EXIT_HOST}:${TARGET_B_PORT}"
api_assert_diag_targets "/api/v1/forward/diagnose" "{\"forwardId\":${FORWARD_LB_ID}}" "${EXIT_HOST}:${TARGET_A_PORT}" "${EXIT_HOST}:${TARGET_B_PORT}"

log "pausing, resuming and updating type1 forward strategy"
api_expect_ok "/api/v1/forward/pause" "{\"id\":${FORWARD_LB_ID}}" >/dev/null
sleep 2
assert_remote_http_unavailable "$ENTRY_HOST" "$ENTRY_FORWARD_PORT"
assert_remote_udp_unavailable "$ENTRY_HOST" "$ENTRY_FORWARD_PORT"
api_expect_ok "/api/v1/forward/resume" "{\"id\":${FORWARD_LB_ID}}" >/dev/null
wait_for_remote_body "$ENTRY_HOST" "$ENTRY_FORWARD_PORT" "DUAL_LB_TARGET_A_209" >/dev/null
api_expect_ok "/api/v1/forward/update" "{\"id\":${FORWARD_LB_ID},\"tunnelId\":${TUNNEL_TYPE1_ID},\"name\":\"dual-lb-forward\",\"remoteAddr\":\"${EXIT_HOST}:${TARGET_A_PORT}, ${EXIT_HOST}:${TARGET_B_PORT}\",\"inPort\":${ENTRY_FORWARD_PORT},\"strategy\":\"random\"}" >/dev/null
sleep 2
selector_strategy="$(agent_json_value "$ENTRY_HOST" "agent-entry-172" "svc=[s for s in d.get('services',[]) if s.get('name')=='${FORWARD_LB_ID}_1_0_tcp'][0]; print(svc['forwarder']['selector']['strategy'])")"
test "$selector_strategy" = "random" || die "expected selector strategy random, got ${selector_strategy}"

log "testing type2 tunnel forward and logical exit switch on 209"
api_expect_ok "/api/v1/tunnel/create" "{\"name\":\"dual-type2\",\"inNodeId\":${NODE_ENTRY_ID},\"outNodeId\":${NODE_EXIT_A_ID},\"type\":2,\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
TUNNEL_TYPE2_ID="$(db_scalar "select id from tunnel where name='dual-type2'")"
api_expect_ok "/api/v1/forward/create" "{\"tunnelId\":${TUNNEL_TYPE2_ID},\"name\":\"dual-type2-forward\",\"remoteAddr\":\"127.0.0.1:${TARGET_TUNNEL_PORT}\",\"inPort\":${TUNNEL_FORWARD_PORT},\"strategy\":\"fifo\"}" >/dev/null
FORWARD_TUNNEL_ID="$(db_scalar "select id from forward where name='dual-type2-forward'")"
sleep 2
tunnel_before="$(wait_for_remote_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "DUAL_TUNNEL_TARGET_209")"
tunnel_udp_before="$(wait_for_remote_udp_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "dual-tunnel-before" "DUAL_UDP_TUNNEL_TARGET_209")"
api_assert_diag_targets "/api/v1/forward/diagnose" "{\"forwardId\":${FORWARD_TUNNEL_ID}}" "127.0.0.1:${TARGET_TUNNEL_PORT}"
assert_contains "$(service_names "$EXIT_HOST" "agent-exit-a-209")" "tunnel_${TUNNEL_TYPE2_ID}_relay"

api_expect_ok "/api/v1/tunnel/update" "{\"id\":${TUNNEL_TYPE2_ID},\"name\":\"dual-type2\",\"inNodeId\":${NODE_ENTRY_ID},\"outNodeId\":${NODE_EXIT_B_ID},\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
tunnel_after_exit="$(wait_for_remote_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "DUAL_TUNNEL_TARGET_209")"
tunnel_udp_after_exit="$(wait_for_remote_udp_body "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT" "dual-tunnel-after-exit" "DUAL_UDP_TUNNEL_TARGET_209")"
chain_after_exit="$(agent_json_value "$ENTRY_HOST" "agent-entry-172" "print(d.get('chains') or [])")"
assert_contains "$chain_after_exit" "${EXIT_HOST}:37510"
assert_not_contains "$(service_names "$EXIT_HOST" "agent-exit-a-209")" "tunnel_${TUNNEL_TYPE2_ID}_relay"
assert_contains "$(service_names "$EXIT_HOST" "agent-exit-b-209")" "tunnel_${TUNNEL_TYPE2_ID}_relay"

log "switching type2 entry from 172 to 209"
api_expect_ok "/api/v1/tunnel/update" "{\"id\":${TUNNEL_TYPE2_ID},\"name\":\"dual-type2\",\"inNodeId\":${NODE_ENTRY_B_ID},\"outNodeId\":${NODE_EXIT_B_ID},\"flow\":2,\"protocol\":\"tls\",\"trafficRatio\":\"1\",\"tcpListenAddr\":\"0.0.0.0\",\"udpListenAddr\":\"0.0.0.0\"}" >/dev/null
wait_for_service_absent "$ENTRY_HOST" "agent-entry-172" "${FORWARD_TUNNEL_ID}_1_0_tcp"
wait_for_chain_absent "$ENTRY_HOST" "agent-entry-172" "tunnel_${TUNNEL_TYPE2_ID}_chains"
wait_for_service_present "$EXIT_HOST" "agent-entry-209" "${FORWARD_TUNNEL_ID}_1_0_tcp"
wait_for_chain_present "$EXIT_HOST" "agent-entry-209" "tunnel_${TUNNEL_TYPE2_ID}_chains"
assert_remote_http_unavailable "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT"
assert_remote_udp_unavailable "$ENTRY_HOST" "$TUNNEL_FORWARD_PORT"
tunnel_after_entry="$(wait_for_remote_body "$EXIT_HOST" "$TUNNEL_FORWARD_PORT" "DUAL_TUNNEL_TARGET_209")"
tunnel_udp_after_entry="$(wait_for_remote_udp_body "$EXIT_HOST" "$TUNNEL_FORWARD_PORT" "dual-tunnel-after-entry" "DUAL_UDP_TUNNEL_TARGET_209")"

log "testing http/ws-compatible config update"
api_expect_ok "/api/v1/node/update" "{\"id\":${NODE_ENTRY_ID},\"name\":\"dual-entry-172\",\"ip\":\"${ENTRY_HOST}\",\"serverIp\":\"${ENTRY_HOST}\",\"portRanges\":\"${ENTRY_PORT_RANGES}\",\"http\":1,\"tls\":0,\"socks\":0}" >/dev/null
wait_for_agent_config_value "$ENTRY_HOST" "agent-entry-172" "http" "1"
api_expect_ok "/api/v1/node/update" "{\"id\":${NODE_ENTRY_ID},\"name\":\"dual-entry-172\",\"ip\":\"${ENTRY_HOST}\",\"serverIp\":\"${ENTRY_HOST}\",\"portRanges\":\"${ENTRY_PORT_RANGES}\",\"http\":0,\"tls\":0,\"socks\":0}" >/dev/null
wait_for_agent_config_value "$ENTRY_HOST" "agent-entry-172" "http" "0"

log "final smoke"
printf '[dual-e2e] load-balance responses:\n%s\n' "$lb_result"
printf '[dual-e2e] load-balance UDP response:\n%s\n' "$lb_udp_result"
printf '[dual-e2e] tunnel before switch:\n%s\n' "$tunnel_before"
printf '[dual-e2e] tunnel UDP before switch:\n%s\n' "$tunnel_udp_before"
printf '[dual-e2e] tunnel after exit switch:\n%s\n' "$tunnel_after_exit"
printf '[dual-e2e] tunnel UDP after exit switch:\n%s\n' "$tunnel_udp_after_exit"
printf '[dual-e2e] tunnel after entry switch:\n%s\n' "$tunnel_after_entry"
printf '[dual-e2e] tunnel UDP after entry switch:\n%s\n' "$tunnel_udp_after_entry"
log "PASS"
