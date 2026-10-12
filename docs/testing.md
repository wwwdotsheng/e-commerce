# 接口与集成测试

## 两层验证

| 层次 | 入口 | 依赖 | 验证内容 |
|---|---|---|---|
| 集成测试 | Gin Router 与 Service | Testcontainers 临时 PostgreSQL、Redis、RabbitMQ | 业务码、数据库状态、事务、并发、幂等和真实 TTL 消费 |
| 部署验收 | Kubernetes Ingress HTTP | 已部署服务 | 健康检查、认证、刷新、重复充值、商品与优惠券下单、订单隔离、超时返还与再次下单 |

普通业务错误也使用 HTTP 200；测试同时检查响应中的业务 code，不能只断言 HTTP 状态。
集成测试默认不访问 Kubernetes，验收测试默认跳过，只有显式指定目标后才执行。

## 在 Ubuntu 服务器运行

工作目录：
/home/w-ubuntu/GolandProjects/experiments/e-commerce

服务器无需安装 Go；脚本使用 Go 1.26 容器，需要 Docker、Bash 和 docker.sock 权限。
依赖镜像前缀按运行环境设置，不写死在测试代码中。

### 集成测试（静态检查、race、乱序）

    IMAGE_REPOSITORY=harbor.internal/dockerhub/ bash scripts/test-integration.sh

输出：
- tmp/test-results/integration.log
- tmp/test-results/integration.xml（JUnit）

有本机 Go 与 Docker 时，也可运行：

    go vet ./...
    go test -race ./internal/order -count=1
    APP_OTEL_ENABLED=false go test -race ./tests -count=1 -timeout=5m -ginkgo.randomize-all

不要使用 -parallel 启动多个 Ginkgo 进程：现有旧用例包含固定账号和共享测试环境。
并发场景是在单个用例内创建 goroutine，校验成功数、订单数、余额和库存日志。

### Kubernetes 只读入口验收

    API_TEST_BASE_URL=http://192.168.20.21:30080 \
    API_TEST_HOST=ecommerce.k8s.internal \
    bash scripts/test-api.sh

只执行健康、就绪、OpenAPI 和非法 Token 校验；写入链路会明确显示 SKIP。

### Kubernetes 完整业务链路

    API_TEST_BASE_URL=http://192.168.20.21:30080 \
    API_TEST_HOST=ecommerce.k8s.internal \
    API_TEST_ALLOW_WRITES=1 \
    bash scripts/test-api.sh

这会创建本次运行专属的 e2e_ 前缀账号、钱包记录、商品、优惠券和订单。
不读取真实用户口令，不操作已有用户数据。
结束时下架本次商品、退出测试会话；账号、优惠券、钱包流水和订单没有删除接口，
因此保留用于排障。测试日志记录运行前缀，不打印登录 Token。

目标必须是明确选择的开发/测试环境；勿对正式用户环境开启写入验收。
订单超时使用轮询，集群等待上限 75 秒。默认 TTL 是 30 秒，
完整链路包括两次下单及超时，通常需要约一分钟。
临时 MQ 集成测试使用独立队列和 100ms TTL，不修改集群 TTL。

## 回归场景

新增订单与钱包集成场景：
- 买完剩余库存时，下单成功；重放只产生一笔订单。
- 并发下单成功数等于库存，数据库中不超卖。
- 他人优惠券校验失败后库存回滚。
- 核销记录关联真实订单 UUID。
- 同一优惠券订单重放成功，不重复扣库存。
- 超时关闭、返库存、退券在同一事务中提交。
- 并发和重复超时只返一次库存，日志也只有一条。
- 已完成订单不会退券或返库存。
- 旧订单超时消息不会退还已被新订单使用的优惠券。
- 退券 SQL 故障注入后，订单与库存状态整体回滚。
- 幂等键不允许跨用户或更换请求参数使用。
- 订单列表只返回当前用户数据。
- 缺失、伪造、签名正确但过期的 Token，以及登出后的会话均不能访问。
- 并发重复充值只入账一次，并断言余额与流水数量。
- 真实 RabbitMQ 延迟/死信队列触发关单与库存释放。

MQ 单元测试覆盖首次成功、暂时失败恢复、达到重试上限、非法消息与上下文取消。
每次投递最多执行一次初始处理加三次重试；取消时重新入队。
重试次数在当前投递内计算，进程重启后可能重新投递；业务事务保证处理幂等。
超过重试上限拒绝消息；现有消费队列没有额外失败队列，不应把它描述为完整消息补偿方案。

## CI 自动部署与验收

product 分支自动执行：test → build-image → api-acceptance。
验收任务先将 ecommerce-api 更新为 IMAGE_REPO:CI_COMMIT_SHORT_SHA，
等待滚动更新成功并核对镜像，再执行完整 HTTP 业务链路。
部署或验收失败都会令流水线失败，日志保存在 test-results/。
一个 resource_group 锁覆盖部署和验收全过程，避免两个流水线在验收时相互更新服务。
其他分支不会自动部署到这个测试环境。

GitLab 变量：
- API_TEST_IMAGE：包含 Go 与 kubectl 的工具镜像。
- KUBE_CONFIG：file 类型，仅限 ecommerce-test 环境。
- API_TEST_BASE_URL / API_TEST_HOST：该环境的服务入口。
- IMAGE_REPO：构建任务实际发布的业务镜像仓库。

k8s/ci/rbac.yaml 是管理员执行一次的权限引导配置，不包含凭据值。
它授予 ecommerce-ci 更新指定 API Deployment、观察 rollout 和读取 Pod 诊断的权限，
不允许读取 Secret、修改数据库 StatefulSet 或修改其他 Deployment。
受限 kubeconfig 已经获得用户明确授权，保存在 GitLab 的测试环境 file 变量中，
不会提交 Git；更换或撤销凭据时更新该变量及对应 ServiceAccount token Secret。

工具镜像源码为 scripts/ci-tools.Dockerfile；构建上下文只需已验证的 kubectl 可执行文件。
scripts/deploy-and-test-api.sh 执行发布、版本校验与验收。
测试失败时保留已部署版本和诊断文件，不自动回退，便于判断业务失败原因。
回退应选择上一成功流水线的镜像，并在测试环境重新执行验收。

这个流程只更新 API 镜像，现有基础设施、ConfigMap、Secret 和 PVC 仍由部署清单管理。
仓库的 harbor-local 固定镜像标签是手工部署基线；CI 会使用本次提交的动态镜像。
手工 apply 整个 overlay 前，必须核对镜像标签，避免恢复旧版本。

## 尚未覆盖

- Elasticsearch 搜索（当前集群未部署）。
- 尚未实现的支付功能。
- Pod 重建、节点故障与持久化恢复演练。
- 数据库提交后 MQ 发布失败的持久化补偿（尚未实现 Outbox）。
- 库存、钱包和订单幂等的业务规则还应由需求文档持续明确。

测试通过表示所列场景通过，不代表所有接口或故障都已覆盖。

## 本次实测结果（2026-10-12）

- go vet ./...：通过。
- 79 个集成用例：通过，包括 17 个新增回归场景；race 与乱序执行通过。
- MQ 单元测试的 5 个场景：通过，并开启 race。
- JUnit XML：0 failures、0 errors；81 个条目包含 BeforeSuite/AfterSuite 两个生命周期条目。
- KubernetesSmoke：通过。
- KubernetesBusinessFlow：通过，约 61 秒；包含两次真实 MQ 超时。
- 部署镜像：harbor.internal/e-commerce/app:fde25f7-testfix-20261012。
- 四个 Pod 均 Ready，集群与 Harbor overlay 无差异。
- CI YAML 与 Bash 脚本语法：本地检查通过，GitLab pipeline 尚未运行。

失败复现和成功结果保存在忽略目录 tmp/test-results/：
invariants-before.log、mq-before.log、acceptance-before.log、integration.xml、integration.log、acceptance.log。
代码、测试和部署说明按主题拆分为独立 Git 提交。

## CI 基础镜像故障排查

流水线 #39 的自动测试通过，build-image 因 Harbor 代理基础镜像不存在而失败。
本地 Docker 缓存可构建，不代表独立 BuildKit daemon 能从远程仓库拉取。

BUILD_REGISTRY_PREFIX 可以单独指定普通 Harbor 项目的构建镜像前缀；
REGISTRY_PREFIX 继续供 Testcontainers 使用，不随构建镜像路径一起改变。
前缀对应的仓库必须包含 library/golang:1.26-alpine 和 library/alpine:latest。
本次将已验证的 Go 1.26.8 / Alpine 3.24.2 镜像发布至普通 Harbor 项目供 amd64 Runner 使用。
后续升级版本时，也应检查远程 manifest，而非仅验证本地 Docker 缓存。

build-image 不下载测试报告，test-results/ 也被排除在 Docker 构建上下文之外。
