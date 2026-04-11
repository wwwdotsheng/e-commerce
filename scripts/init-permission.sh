#!/bin/bash

# 定义数据根目录（根据你的实际路径修改）
DATA_DIR="../data"

echo "开始初始化 Docker 挂载目录权限..."

# 创建目录函数
setup_dir() {
    local path=$1
    local owner=$2
    echo "配置目录: $path (Owner UID: $owner)"
    sudo mkdir -p "$path"
    sudo chown -R "$owner:$owner" "$path"
}

# --- 开始配置各服务目录 ---

# 1. Postgres (UID 999)
setup_dir "$DATA_DIR/postgres" 999

# 2. Redis (UID 999)
setup_dir "$DATA_DIR/redis" 999

# 3. Grafana (UID 472)
setup_dir "$DATA_DIR/grafana" 472

# 4. Prometheus (UID 65534)
setup_dir "$DATA_DIR/prometheus" 65534

# 5. Loki & Tempo (通常使用 10001)
setup_dir "$DATA_DIR/loki-data" 10001
setup_dir "$DATA_DIR/tempo" 10001

# 6. Promtail (通常需要读取宿主机日志，有时需要 root 或特定组，这里先创建目录)
setup_dir "$DATA_DIR/promtail" 10001

echo "--------------------------------------"
echo "权限初始化完成！"
ls -ld $DATA_DIR/*