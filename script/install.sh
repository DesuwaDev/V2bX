#!/bin/bash
#
# Install, update, or put this build in place of the official V2bX (one installed by V2bX-script):
#
#   bash <(curl -Ls https://raw.githubusercontent.com/DesuwaDev/V2bX/dev_new/script/install.sh) [version]
#
# It goes where V2bX-script puts V2bX (/usr/local/V2bX, the V2bX service, the V2bX menu command)
# and keeps /etc/V2bX as it is. The new program is downloaded to a temporary directory, its SHA-256
# checked and run once before anything is replaced; the old one is kept as V2bX.bak and put back
# if the service doesn't stay up with the new one. Nothing is reported anywhere.
#
# Adapted from https://github.com/wyx2685/V2bX-script (MPL-2.0).

REPO="DesuwaDev/V2bX"
SCRIPT_REF="dev_new"
DIR=/usr/local/V2bX
CONF=/etc/V2bX

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

fail() {
    echo -e "${red}错误：${plain}$*" >&2
    exit 1
}

[[ $EUID -ne 0 ]] && fail "必须使用 root 用户运行此脚本！"

# system
if [[ -f /etc/redhat-release ]]; then
    release="centos"
elif cat /etc/issue 2>/dev/null | grep -Eqi "alpine"; then
    release="alpine"
elif cat /etc/issue 2>/dev/null | grep -Eqi "debian"; then
    release="debian"
elif cat /etc/issue 2>/dev/null | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /etc/issue 2>/dev/null | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "debian"; then
    release="debian"
elif cat /proc/version | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /proc/version | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "arch"; then
    release="arch"
else
    fail "未检测到系统版本"
fi

# the release zips there are (.github/workflows/release.yml)
case "$(uname -m)" in
    x86_64 | x64 | amd64) arch="64" ;;
    aarch64 | arm64) arch="arm64-v8a" ;;
    *) fail "不支持的架构：$(uname -m)，目前只有 x86_64 和 arm64 的版本" ;;
esac
[[ $(getconf WORD_BIT) != '32' || $(getconf LONG_BIT) == '64' ]] || fail "本软件不支持 32 位系统，请使用 64 位系统"

install_base() {
    case "$release" in
        centos)
            yum install -y epel-release >/dev/null 2>&1
            yum install -y wget curl unzip tar ca-certificates >/dev/null 2>&1
            ;;
        alpine)
            apk add --no-cache wget curl unzip tar ca-certificates bash coreutils >/dev/null 2>&1
            ;;
        debian | ubuntu)
            apt-get update -y >/dev/null 2>&1
            apt-get install -y wget curl unzip tar ca-certificates >/dev/null 2>&1
            ;;
        arch)
            pacman -Sy --noconfirm --needed wget curl unzip tar ca-certificates >/dev/null 2>&1
            ;;
    esac
    for tool in curl unzip sha256sum; do
        command -v $tool >/dev/null 2>&1 || fail "缺少 $tool，请先安装"
    done
}

service_running() {
    if [[ $release == "alpine" ]]; then
        rc-service V2bX status 2>/dev/null | grep -q started
    else
        systemctl is-active --quiet V2bX
    fi
}

service_stop() {
    if [[ $release == "alpine" ]]; then
        rc-service V2bX stop >/dev/null 2>&1
    else
        systemctl stop V2bX >/dev/null 2>&1
    fi
}

service_start() {
    if [[ $release == "alpine" ]]; then
        rc-service V2bX start >/dev/null 2>&1
    else
        systemctl start V2bX >/dev/null 2>&1
    fi
}

# up and staying up for a few seconds (a broken program exits at once and systemd restarts it)
service_healthy() {
    sleep 2
    service_running || return 1
    sleep 4
    service_running
}

write_service() {
    if [[ $release == "alpine" ]]; then
        cat <<EOF >/etc/init.d/V2bX
#!/sbin/openrc-run

name="V2bX"
description="V2bX"

command="$DIR/V2bX"
command_args="server"
command_user="root"

pidfile="/run/V2bX.pid"
command_background="yes"

depend() {
        need net
}
EOF
        chmod +x /etc/init.d/V2bX
        rc-update add V2bX default >/dev/null 2>&1
    else
        cat <<EOF >/etc/systemd/system/V2bX.service
[Unit]
Description=V2bX Service
After=network.target nss-lookup.target
Wants=network.target

[Service]
User=root
Group=root
Type=simple
LimitAS=infinity
LimitRSS=infinity
LimitCORE=infinity
LimitNOFILE=999999
WorkingDirectory=$DIR/
ExecStart=$DIR/V2bX server
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable V2bX >/dev/null 2>&1
    fi
}

latest_version() {
    # the release page redirects to the latest tag (no API, no rate limit)
    local url
    url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") || return 1
    [[ $url == */releases/tag/* ]] || return 1
    echo "${url##*/}"
}

install_V2bX() {
    local version=$1
    if [[ -z $version ]]; then
        version=$(latest_version) || fail "获取最新版本失败，请确认服务器能访问 GitHub，或指定版本号"
    fi
    echo -e "开始安装 V2bX ${green}${version}${plain}（${REPO}）"

    tmp=$(mktemp -d) || fail "无法创建临时目录"
    trap 'rm -rf "$tmp"' EXIT
    local url="https://github.com/$REPO/releases/download/$version/V2bX-linux-$arch.zip"
    curl -fL --retry 3 --progress-bar -o "$tmp/V2bX.zip" "$url" || fail "下载失败：$url"
    curl -fsL --retry 3 -o "$tmp/V2bX.zip.dgst" "$url.dgst" || fail "下载校验文件失败：$url.dgst"
    local expected actual
    expected=$(grep -iE '^SHA2?-?256' "$tmp/V2bX.zip.dgst" | awk '{print $NF}' | tr 'A-F' 'a-f')
    actual=$(sha256sum "$tmp/V2bX.zip" | awk '{print $1}')
    [[ -n $expected && $expected == "$actual" ]] || fail "SHA-256 校验不通过，已放弃安装"
    unzip -q -o "$tmp/V2bX.zip" -d "$tmp/pkg" || fail "解压失败"
    chmod +x "$tmp/pkg/V2bX"
    "$tmp/pkg/V2bX" version >/dev/null 2>&1 || fail "新程序无法在本机运行，已放弃安装"

    local was_running=false
    service_running && was_running=true
    service_stop
    mkdir -p "$DIR" "$CONF"
    local backup=false
    if [[ -f $DIR/V2bX ]]; then
        cp -f "$DIR/V2bX" "$DIR/V2bX.bak" && backup=true
    fi
    # the files of the zip next to the program, then the program itself in one move
    for f in "$tmp/pkg"/*; do
        [[ $(basename "$f") == "V2bX" ]] || cp -f "$f" "$DIR/"
    done
    cp -f "$tmp/pkg/V2bX" "$DIR/V2bX.new" && mv -f "$DIR/V2bX.new" "$DIR/V2bX" || fail "替换程序失败"
    cp -f "$tmp/pkg/geoip.dat" "$tmp/pkg/geosite.dat" "$CONF/"
    for f in dns.json route.json custom_outbound.json custom_inbound.json; do
        [[ -f $CONF/$f ]] || cp "$tmp/pkg/$f" "$CONF/"
    done
    write_service

    first_install=false
    if [[ ! -f $CONF/config.json ]]; then
        cp "$tmp/pkg/config.json" "$CONF/"
        first_install=true
        echo -e "${green}V2bX ${version}${plain} 安装完成，已设置开机自启。全新安装，请先填写 $CONF/config.json"
    else
        service_start
        if service_healthy; then
            echo -e "${green}V2bX ${version} 已启动${plain}"
        elif [[ $backup == true ]]; then
            echo -e "${red}新程序没能稳定运行，已换回原来的程序${plain}"
            service_stop
            mv -f "$DIR/V2bX.bak" "$DIR/V2bX"
            service_start
            if service_healthy; then
                echo -e "${yellow}原来的程序已恢复运行；请用 V2bX log 查看新程序的报错${plain}"
            else
                echo -e "${red}原来的程序也没能启动，请用 V2bX log 查看日志（可能是配置或面板的问题）${plain}"
            fi
            exit 1
        else
            echo -e "${red}V2bX 没能稳定运行，请用 V2bX log 查看日志${plain}"
            [[ $was_running == true ]] && exit 1
        fi
    fi

    # the menu command, from the same version, so its updates come from here too
    if curl -fsSL -o "$tmp/V2bX.sh" "https://raw.githubusercontent.com/$REPO/$version/script/V2bX.sh" ||
        curl -fsSL -o "$tmp/V2bX.sh" "https://raw.githubusercontent.com/$REPO/$SCRIPT_REF/script/V2bX.sh"; then
        install -m 755 "$tmp/V2bX.sh" /usr/bin/V2bX
        ln -sf /usr/bin/V2bX /usr/bin/v2bx
    else
        echo -e "${yellow}管理脚本下载失败，V2bX 命令保持原样${plain}"
    fi

    echo -e ""
    echo "V2bX 管理脚本使用方法 (兼容使用V2bX执行，大小写不敏感): "
    echo "------------------------------------------"
    echo "V2bX              - 显示管理菜单 (功能更多)"
    echo "V2bX start        - 启动 V2bX"
    echo "V2bX stop         - 停止 V2bX"
    echo "V2bX restart      - 重启 V2bX"
    echo "V2bX status       - 查看 V2bX 状态"
    echo "V2bX enable       - 设置 V2bX 开机自启"
    echo "V2bX disable      - 取消 V2bX 开机自启"
    echo "V2bX log          - 查看 V2bX 日志"
    echo "V2bX x25519       - 生成 x25519 密钥"
    echo "V2bX generate     - 生成 V2bX 配置文件"
    echo "V2bX update       - 更新 V2bX"
    echo "V2bX update x.x.x - 更新 V2bX 指定版本"
    echo "V2bX install      - 安装 V2bX"
    echo "V2bX uninstall    - 卸载 V2bX"
    echo "V2bX version      - 查看 V2bX 版本"
    echo "------------------------------------------"

    if [[ $first_install == true ]]; then
        read -rp "检测到你为第一次安装V2bX,是否自动直接生成配置文件？(y/n): " if_generate
        if [[ $if_generate == [Yy] ]]; then
            curl -fsSL -o "$tmp/initconfig.sh" "https://raw.githubusercontent.com/$REPO/$SCRIPT_REF/script/initconfig.sh" &&
                source "$tmp/initconfig.sh" && generate_config_file
        fi
    fi
}

echo -e "${green}开始安装${plain}"
install_base
install_V2bX "$1"
