# Go 电商后端

基于 Go 的电商后端服务，包含用户、认证、商品、订单、优惠券、钱包、店铺七个模块。

## 技术栈

| 类别 | 技术 |
|---|---|
| 语言 / 框架 | Go 1.25 · Gin · GORM |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 消息队列 | RabbitMQ |
| 检索 | Elasticsearch |
| 可观测性 | OpenTelemetry · Tempo · Prometheus · Loki · Grafana |
| 测试 | Ginkgo · Testcontainers |
| 交付 | Docker Compose · Gitea Actions |

## 快速开始

依赖：Go 1.25+、Docker / Docker Compose。

```bash
# 启动基础设施（PostgreSQL / Redis / RabbitMQ / Elasticsearch 及监控栈）
docker compose up -d

# 启动服务
go run cmd/main.go -c configs/config.yaml
```

服务默认监听 `:8080`。

```bash
curl http://localhost:8080/health    # 健康检查
curl http://localhost:8080/ready     # 就绪检查
```

接口文档：`http://localhost:8080/swagger`

## 配置

配置位于 `configs/config.yaml`。所有配置项均可通过环境变量覆盖，前缀 `APP_`，层级用 `_` 分隔：

```bash
APP_DATABASE_HOST=127.0.0.1 APP_AUTH_TOKEN_SECRET=change-me go run cmd/main.go -c configs/config.yaml
```

| 配置段 | 说明 |
|---|---|
| `app` | 服务名、监听端口、运行环境 |
| `database` | PostgreSQL 连接与连接池 |
| `redis` | Redis 连接与连接池 |
| `rabbitmq` | RabbitMQ 连接 |
| `elasticsearch` | Elasticsearch 地址 |
| `auth` | JWT 有效期与签名密钥 |
| `order_mq` | 订单超时队列、交换机、TTL |
| `otel` | OpenTelemetry 开关与上报端点 |
| `registry` | 镜像仓库前缀 |
| `log` | 日志级别与轮转策略 |

## 项目结构

```
cmd/                    程序入口
internal/
  app/                  启动、路由注册、依赖装配、优雅关闭
  auth/                 JWT 双 Token 与 Redis 会话管理
  user/                 用户注册
  product/              商品 CRUD、库存、搜索
  order/                订单创建、超时关单
  coupon/               优惠券模板与用户券
  wallet/               钱包充值
  shop/                 店铺
  middleware/           认证、限流、日志、链路追踪
  model/                GORM 数据模型
  config/               配置加载与校验
pkg/
  clog/                 结构化日志
  dbconn/               数据库连接
  errno/                错误码
  mq/                   RabbitMQ 封装
  redis/                Redis 封装
  esconn/               Elasticsearch 连接
tests/                  集成测试
configs/                应用配置与监控栈配置
k8s/                    Kubernetes 部署清单
```

## 接口

所有业务接口前缀为 `/api/v1`。除注册和登录外均需在请求头携带 AccessToken。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/user/register` | 注册 |
| POST | `/api/v1/auth/login` | 登录（限流 5 req/s） |
| POST | `/api/v1/auth/fetch-access-token` | 用 RefreshToken 刷新 AccessToken |
| POST | `/api/v1/auth/fetch-refresh-token` | 刷新 RefreshToken |
| POST | `/api/v1/auth/logout` | 登出并清理会话 |
| POST | `/api/v1/product/create` | 创建商品 |
| GET | `/api/v1/product/list` | 商品列表 |
| GET | `/api/v1/product/search` | 商品搜索（Elasticsearch） |
| GET | `/api/v1/product/:id` | 商品详情 |
| PATCH | `/api/v1/product/:id` | 更新商品属性 |
| POST | `/api/v1/product/:id/status` | 更新商品状态 |
| DELETE | `/api/v1/product/:id` | 删除商品 |
| POST | `/api/v1/order/create` | 创建订单 |
| GET | `/api/v1/order/list` | 订单列表 |
| POST | `/api/v1/coupon/template` | 创建优惠券模板 |
| POST | `/api/v1/coupon/grant` | 领取优惠券 |
| GET | `/api/v1/coupon/list` | 我的优惠券 |
| POST | `/api/v1/wallet/deposit` | 钱包充值 |
| POST | `/api/v1/shop/create` | 创建店铺 |
| GET | `/api/v1/shop/list` | 店铺列表 |
| GET | `/api/v1/shop/:id` | 店铺详情 |

## 测试

```bash
go test ./tests/... -v
```

集成测试基于 Ginkgo 与 Testcontainers，运行前自动创建 PostgreSQL、Redis、RabbitMQ 容器，运行后销毁，不依赖本地环境。

CI 在 Gitea Actions 上执行，开启 `--race` 竞态检测与 `--randomize-all` 用例乱序执行。

## 可观测性

| 类型 | 导出方式 | 后端 |
|---|---|---|
| Trace | OTLP gRPC | Tempo |
| Metric | OTLP HTTP | Prometheus |
| Log | JSON stdout → promtail | Loki |

日志自动携带 `trace_id` 与 `span_id`，可从链路直接关联到日志。Grafana 地址 `http://localhost:3000`。

## 部署

Kubernetes 清单位于 `k8s/`，包含 namespace、configmap、ingress，以及应用、PostgreSQL、Redis、RabbitMQ 的 deployment、service 与 pvc。

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/
```

## License

MIT
