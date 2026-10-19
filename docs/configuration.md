# 配置与部署

应用只有一套配置结构，不根据 Dev/Prod/Test 等名称改变行为。
AppEnv / app.env / APP_APP_ENV 已移除；旧名称出现时会明确报错，避免误以为仍然生效。

## 来源与优先级

进程环境变量 > 显式指定的 dotenv > YAML > 代码默认值。

- YAML：-c 参数或 CONFIG_PATH。未指定时尝试 configs/config.yaml；
  默认文件不存在可使用纯环境变量，显式指定但不存在的文件会报错。
- dotenv：-dotenv 参数或 DOTENV_PATH，仅在明确指定时读取。
  不会悄悄加载工作目录 .env，也不会改写进程环境变量。
- APP_ 前缀，配置层级用下划线分隔。
- APP_AUTH_ACCESS_TOKEN_EXPIRE=15m 等时长必须写单位。
- APP_REGISTRY_PREFIX / APP_APP_GIN_MODE 为推荐名称。
  IMAGE_REPOSITORY / GIN_MODE 为兼容别名，推荐名称优先。
- YAML 未出现的字段也可由环境变量覆盖；空字符串是有效覆盖值，
  必填字段被覆盖为空后会校验失败。
- YAML 语法错误、未知字段、非法端口、错误布尔值、连接池参数和缺少依赖配置会阻止启动。

## 显式行为开关

| 配置项 | 默认 | 含义 |
|---|---|---|
| otel.enabled | false | 是否启用链路和指标导出 |
| elasticsearch.enabled | false | 是否初始化并使用搜索依赖 |
| app.print_config | false | 打印脱敏配置，永远不打印密码和 JWT Secret |
| app.expose_dev_msg | false | 响应是否包含内部错误详情 |
| app.ssl | false | HTTP 服务是否使用证书 |
| app.gin_mode | release | Gin 的日志/调试模式，不代表部署环境 |
| log.file_level + log.filename | 空 | 两者配置后才写日志文件；默认 stdout |

启用 OTel 必须提供 trace 和 metric endpoint。
启用 Elasticsearch 必须配置可连接的地址；初始化失败会阻止应用启动，
不会把“显式启用但依赖不可用”默默当成关闭。
关闭这些功能时不会尝试连接它们。

订单 TTL 本次保持 30000 毫秒（30 秒），不是 30 分钟。
修改 TTL 时必须迁移 RabbitMQ 已声明队列，不能直接用不同参数重声明旧队列。
database.conn_max_lifetime 保留原有整数语义，单位为分钟；此轮未改变它的运行含义。

## Docker Compose

需要 Docker Compose 2.24+（支持可选 env_file）和 Docker。

1. 将 .env.example 复制为 .env；将 JWT 示例值替换为随机签名密钥。
2. 填写数据库和 RabbitMQ 密码；同一变量分别注入应用和对应依赖容器。
3. 先启动 PostgreSQL，再按数据库迁移指南运行 Atlas 迁移。
4. 执行 docker compose up -d --build --wait。
5. 检查 http://localhost:10000/ready 和 /swagger。

Compose 自动读取 .env 用于插值和 env_file；这是 Compose 提供环境变量，
不是应用判断某个环境名称。未列在 environment 中的 APP_* 也可通过 .env 注入。
进程环境变量可以覆盖 Compose 文件中显式列出的插值参数。

核心栈仅启动 API、PostgreSQL、Redis、RabbitMQ。
APP_HTTP_PORT / POSTGRES_PORT / REDIS_PORT / RABBITMQ_PORT 控制对主机暴露的端口。
内部服务地址与端口由 Compose 拓扑配置；这些不应使用主机端口。
容器不再使用固定名称，数据使用 Compose 项目独立命名卷。

旧配置的数据目录、postgres-dev 容器等不会被自动迁移或删除。
已有数据需另行备份/迁移，勿用 down -v 清理实际业务数据。

可选功能：
- 搜索：APP_ELASTICSEARCH_ENABLED=true docker compose --profile search up -d
- 监控：APP_OTEL_ENABLED=true docker compose --profile observability up -d
- 文件日志采集：额外设置 APP_LOG_FILE_LEVEL=info、
  APP_LOG_FILENAME=/app/logs/app.log；promtail 读取挂载的日志目录。
- 热重载：docker compose -f docker-compose.yaml -f docker-compose.watch.yaml up -d --build

profiles 只控制是否启动功能依赖；具体应用行为仍由配置开关决定，
不会根据是否启动某个容器来猜测环境。

## 直接运行

APP_DATABASE_PASSWORD=... APP_RABBITMQ_PASSWORD=... APP_AUTH_TOKEN_SECRET=... \
go run ./cmd -c configs/config.yaml

或显式读取本地凭据文件：

go run ./cmd -c configs/config.yaml -dotenv .env

默认地址 localhost；依赖在远程机器时，用对应 APP_*_HOST / PORT 覆盖。

## Kubernetes

Docker Compose 使用 configs/config.yaml；Kubernetes 不挂载或读取应用 YAML 配置文件。
k8s/app/configmap.yaml 将普通配置逐项写成 APP_* 键，Deployment 通过 envFrom 注入。
敏感密码和 JWT 签名密钥继续从 Secret 引用，映射为 APP_DATABASE_PASSWORD、
APP_REDIS_PASSWORD、APP_RABBITMQ_PASSWORD、APP_AUTH_TOKEN_SECRET。

例如 app.gin_mode 对应 APP_APP_GIN_MODE，database.host 对应
APP_DATABASE_HOST。Secret 内部 key 可以继续使用 POSTGRES_PASSWORD 等名称；
Deployment 的 env.name 才是应用读取的 APP_* 环境变量名。

Deployment 显式设置 CONFIG_PATH 为空，表示只读取环境变量。CONFIG_PATH 未设置时，
直接运行和 Docker 仍可使用默认 configs/config.yaml。Kubernetes 的 env-only 启动仍
使用相同默认值、类型转换和 Validate()；必填凭据由 Secret 提供。

ConfigMap 通过 envFrom 注入。变更 ConfigMap 后需重建 Pod，环境变量不会在运行时更新。
自动发布流水线只更新 API 镜像，不自动修改依赖、ConfigMap 或 Secret。
先审核并应用配置变更，再发布应用；手工操作前核对镜像标签避免覆盖 CI 版本。

## 验证

配置单元与 MQ 测试：
go test -race ./internal/config ./internal/order

完整隔离集成测试：
IMAGE_REPOSITORY=你的测试镜像前缀 bash scripts/test-integration.sh

Compose 验证使用独立项目、随机主机端口和独立命名卷，不操作现有容器的数据。
Kubernetes 验收继续由 product 的自动发布流水线执行。

## 本次验证（2026-10-12）

- 配置单元测试、MQ 单元测试、go vet 与 race：通过。
- 全部 79 个集成用例：通过，开启乱序与 race。
- Compose runtime 与 watch override：配置解析通过。
- 独立 Compose 项目：核心服务健康、密码一致注入、脱敏诊断与完整 HTTP 业务验收通过。
- 独立 Kubernetes 命名空间：ConfigMap/Secret 注入、四个工作负载、PVC 与完整业务验收通过。
- 测试项目、数据卷及命名空间已经清理，原有容器与 ecommerce 服务未修改。
- 可选搜索/监控配置通过加载和条件校验；本次未启动完整搜索与监控栈。

详细日志保存在忽略目录 tmp/test-results/，不会随代码提交。
