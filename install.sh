#!/bin/bash
set -Eeuo pipefail

get_architecture() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "amd64" ;;
  esac
}

build_download_url() {
  local arch
  arch="$(get_architecture)"
  echo "https://minio.uily.de/files/flux-agent/new-flux-agent-linux-${arch}"
}

DOWNLOAD_URL="${DOWNLOAD_URL:-$(build_download_url)}"
INSTALL_DIR="${INSTALL_DIR:-/etc/gost_flux}"
SERVICE_NAME="${SERVICE_NAME:-gost_flux}"
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
SYSTEMCTL_BIN="${SYSTEMCTL_BIN:-systemctl}"
SKIP_TCPKILL_INSTALL="${SKIP_TCPKILL_INSTALL:-0}"
SKIP_DELETE_SELF="${SKIP_DELETE_SELF:-0}"
AUTO_CONFIRM="${AUTO_CONFIRM:-0}"
START_SERVICE="${START_SERVICE:-1}"
SERVER_ADDR="${SERVER_ADDR:-}"
SECRET="${SECRET:-}"

show_menu() {
  echo "==============================================="
  echo "              管理脚本"
  echo "==============================================="
  echo "请选择操作："
  echo "1. 安装"
  echo "2. 更新"
  echo "3. 卸载"
  echo "4. 退出"
  echo "==============================================="
}

delete_self() {
  if [[ "$SKIP_DELETE_SELF" == "1" ]]; then
    return 0
  fi
  echo ""
  echo "🗑️ 操作已完成，正在清理脚本文件..."
  local script_path
  script_path="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
  sleep 1
  rm -f "$script_path" && echo "✅ 脚本文件已删除" || echo "❌ 删除脚本文件失败"
}

check_and_install_tcpkill() {
  if [[ "$SKIP_TCPKILL_INSTALL" == "1" ]]; then
    return 0
  fi
  if command -v tcpkill >/dev/null 2>&1; then
    return 0
  fi

  local sudo_cmd=""
  if [[ ${EUID:-0} -ne 0 ]]; then
    sudo_cmd="sudo"
  fi

  if [[ "$(uname -s)" == "Darwin" ]]; then
    if command -v brew >/dev/null 2>&1; then
      brew install dsniff >/dev/null 2>&1 || true
    fi
    return 0
  fi

  local distro=""
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    distro="${ID:-}"
  elif [[ -f /etc/redhat-release ]]; then
    distro="rhel"
  elif [[ -f /etc/debian_version ]]; then
    distro="debian"
  fi

  case "$distro" in
    ubuntu|debian)
      $sudo_cmd apt update >/dev/null 2>&1 || true
      $sudo_cmd apt install -y dsniff >/dev/null 2>&1 || true
      ;;
    centos|rhel|fedora)
      if command -v dnf >/dev/null 2>&1; then
        $sudo_cmd dnf install -y dsniff >/dev/null 2>&1 || true
      elif command -v yum >/dev/null 2>&1; then
        $sudo_cmd yum install -y dsniff >/dev/null 2>&1 || true
      fi
      ;;
    alpine) $sudo_cmd apk add --no-cache dsniff >/dev/null 2>&1 || true ;;
    arch|manjaro) $sudo_cmd pacman -S --noconfirm dsniff >/dev/null 2>&1 || true ;;
    opensuse*|sles) $sudo_cmd zypper install -y dsniff >/dev/null 2>&1 || true ;;
    gentoo) $sudo_cmd emerge --ask=n net-analyzer/dsniff >/dev/null 2>&1 || true ;;
    void) $sudo_cmd xbps-install -Sy dsniff >/dev/null 2>&1 || true ;;
  esac
}

get_config_params() {
  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    echo "请输入配置参数："
    if [[ -z "$SERVER_ADDR" ]]; then
      read -r -p "服务器地址: " SERVER_ADDR
    fi
    if [[ -z "$SECRET" ]]; then
      read -r -p "密钥: " SECRET
    fi
  fi

  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    echo "❌ 参数不完整，操作取消。"
    exit 1
  fi
}

service_exists() {
  "$SYSTEMCTL_BIN" list-units --full -all 2>/dev/null | grep -Fq "${SERVICE_NAME}.service"
}

install_gost() {
  echo "🚀 开始安装 GOST..."
  get_config_params
  check_and_install_tcpkill

  mkdir -p "$INSTALL_DIR"

  if service_exists; then
    echo "🔍 检测到已存在的 gost 服务"
    "$SYSTEMCTL_BIN" stop "$SERVICE_NAME" 2>/dev/null && echo "🛑 停止服务" || true
    "$SYSTEMCTL_BIN" disable "$SERVICE_NAME" 2>/dev/null && echo "🚫 禁用自启" || true
  fi

  [[ -f "$INSTALL_DIR/gost_flux" ]] && echo "🧹 删除旧文件 gost_flux" && rm -f "$INSTALL_DIR/gost_flux"

  echo "⬇️ 下载 gost 中..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/gost_flux"
  if [[ ! -f "$INSTALL_DIR/gost_flux" || ! -s "$INSTALL_DIR/gost_flux" ]]; then
    echo "❌ 下载失败，请检查网络或下载链接。"
    exit 1
  fi
  chmod +x "$INSTALL_DIR/gost_flux"
  echo "✅ 下载完成"

  echo "🔎 gost 版本：$("$INSTALL_DIR/gost_flux" -V 2>/dev/null || echo unknown)"

  local config_file="$INSTALL_DIR/config.json"
  echo "📄 创建新配置: config.json"
  cat > "$config_file" <<EOF
{
  "addr": "$SERVER_ADDR",
  "secret": "$SECRET"
}
EOF

  local gost_config="$INSTALL_DIR/gost.json"
  if [[ -f "$gost_config" ]]; then
    echo "⏭️ 跳过配置文件: gost.json (已存在)"
  else
    echo "📄 创建新配置: gost.json"
    cat > "$gost_config" <<EOF
{}
EOF
  fi
  chmod 600 "$INSTALL_DIR"/*.json

  mkdir -p "$SYSTEMD_DIR"
  local service_file="$SYSTEMD_DIR/${SERVICE_NAME}.service"
  cat > "$service_file" <<EOF
[Unit]
Description=Gost Proxy Service
After=network.target

[Service]
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/gost_flux
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

  "$SYSTEMCTL_BIN" daemon-reload
  "$SYSTEMCTL_BIN" enable "$SERVICE_NAME"
  if [[ "$START_SERVICE" == "1" ]]; then
    "$SYSTEMCTL_BIN" start "$SERVICE_NAME"
  fi

  echo "🔄 检查服务状态..."
  if [[ "$START_SERVICE" != "1" ]]; then
    echo "✅ 安装完成，服务启动已跳过。"
    echo "📁 配置目录: $INSTALL_DIR"
  elif "$SYSTEMCTL_BIN" is-active --quiet "$SERVICE_NAME"; then
    echo "✅ 安装完成，gost服务已启动并设置为开机启动。"
    echo "📁 配置目录: $INSTALL_DIR"
    echo "🔧 服务状态: $("$SYSTEMCTL_BIN" is-active "$SERVICE_NAME")"
  else
    echo "❌ gost服务启动失败，请执行以下命令查看日志："
    echo "journalctl -u $SERVICE_NAME -f"
  fi
}

update_gost() {
  echo "🔄 开始更新 GOST..."

  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "❌ GOST 未安装，请先选择安装。"
    return 1
  fi

  echo "📥 使用下载地址: $DOWNLOAD_URL"
  check_and_install_tcpkill

  echo "⬇️ 下载最新版本..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/gost_flux.new"
  if [[ ! -f "$INSTALL_DIR/gost_flux.new" || ! -s "$INSTALL_DIR/gost_flux.new" ]]; then
    echo "❌ 下载失败。"
    return 1
  fi

  if service_exists; then
    echo "🛑 停止 gost 服务..."
    "$SYSTEMCTL_BIN" stop "$SERVICE_NAME"
  fi

  mv "$INSTALL_DIR/gost_flux.new" "$INSTALL_DIR/gost_flux"
  chmod +x "$INSTALL_DIR/gost_flux"
  echo "🔎 新版本：$("$INSTALL_DIR/gost_flux" -V 2>/dev/null || echo unknown)"

  echo "🔄 重启服务..."
  if [[ "$START_SERVICE" == "1" ]]; then
    "$SYSTEMCTL_BIN" start "$SERVICE_NAME"
  else
    echo "⏭️ 跳过服务启动"
  fi

  echo "✅ 更新完成。"
}

uninstall_gost() {
  echo "🗑️ 开始卸载 GOST..."

  local confirm=""
  if [[ "$AUTO_CONFIRM" == "1" ]]; then
    confirm="y"
  else
    read -r -p "确认卸载 GOST 吗？此操作将删除所有相关文件 (y/N): " confirm
  fi
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  if service_exists; then
    echo "🛑 停止并禁用服务..."
    "$SYSTEMCTL_BIN" stop "$SERVICE_NAME" 2>/dev/null || true
    "$SYSTEMCTL_BIN" disable "$SERVICE_NAME" 2>/dev/null || true
  fi

  local service_file="$SYSTEMD_DIR/${SERVICE_NAME}.service"
  if [[ -f "$service_file" ]]; then
    rm -f "$service_file"
    echo "🧹 删除服务文件"
  fi

  if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  "$SYSTEMCTL_BIN" daemon-reload
  echo "✅ 卸载完成"
}

while getopts "a:s:" opt; do
  case "$opt" in
    a) SERVER_ADDR="$OPTARG" ;;
    s) SECRET="$OPTARG" ;;
    *) echo "❌ 无效参数"; exit 1 ;;
  esac
done

main() {
  if [[ -n "$SERVER_ADDR" && -n "$SECRET" ]]; then
    install_gost
    delete_self
    exit 0
  fi

  while true; do
    show_menu
    read -r -p "请输入选项 (1-4): " choice

    case "$choice" in
      1)
        install_gost
        delete_self
        exit 0
        ;;
      2)
        update_gost
        delete_self
        exit 0
        ;;
      3)
        uninstall_gost
        delete_self
        exit 0
        ;;
      4)
        echo "👋 退出脚本"
        delete_self
        exit 0
        ;;
      *)
        echo "❌ 无效选项，请输入 1-4"
        echo ""
        ;;
    esac
  done
}

main
