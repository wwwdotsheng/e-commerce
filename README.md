# Go 电商后端

基于 Go 的电商后端服务，包含用户、认证、商品、订单、优惠券、钱包六个模块。

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
| 交付 | Docker Compose · Kubernetes · GitLab CI |

## 快速开始

使用 Docker Compose（核心栈：API、PostgreSQL、Redis、RabbitMQ）：

1. 复制 .env.example 为 .env，替换 JWT 签名密钥并填写依赖密码。
2. 启动 PostgreSQL：docker compose up -d --wait postgres。
3. 按[数据库迁移](docs/design/configuration.md#数据库迁移)配置 `DATABASE_URL` 并应用待执行的迁移。
4. 启动应用及其他依赖：docker compose up -d --build --wait。

API： http://localhost:10000
健康/就绪： /health、/ready
接口文档： /swagger

可选搜索、监控和热重载按功能启用。应用不区分 Dev/Prod/Test 环境名称。

## 配置

配置来源优先级：进程环境变量 > 显式 dotenv > YAML > 默认值。
-c / CONFIG_PATH 指定 YAML；-dotenv / DOTENV_PATH 显式指定 dotenv。
APP_ 环境变量可覆盖所有配置字段，包括 YAML 未出现的字段。
Docker Compose 使用 YAML 配置文件；Kubernetes 用 ConfigMap 注入普通 APP_* 环境变量、Secret 注入敏感值，不挂载应用配置文件。
密码与 JWT Secret 没有代码默认值；错误或未知配置会阻止启动。

配置项和部署方式见[配置与部署](docs/design/configuration.md)。

## 项目结构

```
cmd/                    程序入口
internal/
  app/                  启动、路由注册、依赖装配、优雅关闭
  auth/                 JWT 双 Token 与 Redis 会话管理
  user/                 用户注册
  product/              商品 CRUD、库存、搜索
  order/                订单创建、详情、余额支付、超时关单
  coupon/               优惠券模板与用户券
  wallet/               钱包充值、余额查询与订单资金流水
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
| GET | `/api/v1/order/:id` | 订单详情（包含退款状态字段） |
| POST | `/api/v1/order/:id/pay` | 使用账户余额支付订单 |
| POST | `/api/v1/coupon/template` | 创建优惠券模板 |
| POST | `/api/v1/coupon/grant` | 领取优惠券 |
| GET | `/api/v1/coupon/list` | 我的优惠券 |
| POST | `/api/v1/wallet/deposit` | 钱包充值 |
| GET | `/api/v1/wallet/balance` | 查询当前账号余额 |

## 测试

```bash
go test ./tests/... -v
```

集成测试基于 Ginkgo 与 Testcontainers，运行前自动创建 PostgreSQL、Redis、RabbitMQ 容器，运行后销毁，不依赖本地环境。

CI 在 GitLab CI 上执行（`test` → `build` 推镜像），开启 `--race` 竞态检测与 `--randomize-all` 用例乱序执行。

接口验收、并发/超时回归、Docker 测试脚本与 CI 报告说明见 [测试说明](docs/design/testing.md)。

## 可观测性

| 类型 | 导出方式 | 后端 |
|---|---|---|
| Trace | OTLP gRPC | Tempo |
| Metric | OTLP HTTP | Prometheus |
| Log | JSON stdout → promtail | Loki |

日志自动携带 `trace_id` 与 `span_id`，可从链路直接关联到日志。Grafana 地址 `http://localhost:3000`。

## 部署

Kubernetes 清单位于 `k8s/`，使用 Kustomize 管理基础配置和部署环境。

基础配置适合本地或开发环境：

```bash
kubectl apply -k k8s
```

生产环境模板位于 `k8s/overlays/production/`。使用前请根据自己的镜像仓库和域名修改其中的示例值：

```bash
kubectl diff -k k8s/overlays/production
kubectl apply -k k8s/overlays/production
```

仓库中的 Secret 仅用于开发环境示例。生产环境应替换为随机密码和 JWT Secret，并通过 Secret 管理系统或集群外部注入，避免将真实凭据提交到 Git。

## License

MIT

认证使用 43 字符的安全随机令牌，由 Redis 保存会话并校验有效期。由 JWT 升级后需重新登录。`APP_AUTH_TOKEN_SECRET` 仅保留配置兼容性，不再参与鉴权；访问令牌和刷新令牌的有效期仍可分别配置。

商家账号审核通过后即可直接发布商品，无需独立创建店铺；商品及订单通过 `seller_id` 关联商家。商家资料使用 `seller_name`、`application_note`，详见 `docs/requirements/user-registration-review.md`。

订单支付使用买家账户余额，支付成功即把订单金额记入商家余额；重复支付、余额不足、超时后的处理规则见 `docs/requirements/order-payment.md`。
