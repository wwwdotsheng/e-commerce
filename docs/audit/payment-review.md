# 余额支付审计

审计对象：`feat/order-payment` @ `ae75d76`（支付实现 `9837010`），只读审计，未修改任何业务代码。
关注：`POST /api/v1/order/:id/pay`（`internal/order/service.go:238-301`）。

## 结论摘要（5 行内）

1. **P0 = 0**：核心资金路径「锁订单行 → 校验状态/截止时间 → 条件扣款 → 等额入账 → 置已支付」在**同一事务、同一连接**内完成，未发现重复扣款、丢失更新、单边记账或支付/超时双向竞态导致的资金不一致。
2. **P1 = 1**：`wallet_logs.idempotency_key` 是**全局唯一**命名空间，被系统键（`order:<id>:payment|income`）与**用户可控的充值幂等键**共用；一旦冲突，兜底路径退化为**未映射的 500**，订单永久不可支付且无恢复手段（`docs/order-payment.md:18` 把它描述成"幂等兜底"，与实际行为不符）。
3. **P2 = 8**：集中在防御性校验、错误码映射、金额类型、账本可追溯性、契约文档与滚动发布顺序，均不阻断上线但会带来误导性告警/对账困难。
4. 三条审计线的资金侧结论：锁顺序一致且无死锁环；买家扣款与商家入账**恒等**；`payable_amount` 快照语义正确且历史订单已回填，**不存在 NULL 金额白拿订单的窗口**。
5. 未执行 `tests/` 下的 Testcontainers 集成测试（本机无 Docker）；并发/竞态结论由代码路径推理 + 现有用例对照得出，见文末「验证记录」。

## P0 / P1 / P2 问题清单

### P0（0 条）

未发现资金损失、可重复扣款、越权支付或跨事务半提交问题。判定依据见「已核对无问题的点」第 1–6 条。

### P1（1 条）

**P1-1 幂等键命名空间混用：充值键与订单键共用全局唯一索引，冲突时支付退化为永久 500**

- 现象：`wallet_logs` 的唯一索引是 `unique(idempotency_key)`（全局，不分用户、不分类型），订单支付固定写入 `order:<orderID>:payment` / `order:<orderID>:income`（`internal/wallet/repository.go:158-165`），而充值接口的幂等键完全由调用方提供且**只校验非空**（`internal/wallet/handler.go:14-26`，无 max 长度、无前缀白名单；51 字符的订单键可放入 `varchar(64)`）。只要该键先被任何一条流水的 INSERT 占用，支付时 `createLog` 会拿到 23505 → `repoErrWalletLogAlreadyExists`（未导出错误，`internal/wallet/repository.go:19-27,144-156`）。
- 代码位置：`internal/wallet/repository.go:82-110`（先写流水的扣款）、`113-142`（入账）、`144-156`（冲突映射）；`internal/order/service.go:287-299`（只映射 4 个业务错误）；`internal/wallet/service.go:52-53`（充值路径把同一错误当成功，可对比）；`migrations/20260507035237.sql:65`。
- 依据（可达路径）：买家 `GET /api/v1/order/list` 拿到自己的订单 ID → `POST /api/v1/wallet/deposit {"amount":0.01,"idempotency_key":"order:<id>:payment"}` 成功（`bootstrap.go:141-143`，任意登录账号可用）→ 再 `POST /order/:id/pay`：`createLog` 唯一冲突，`PayOrder` 无法识别该错误 → `response.Write` 落到 `B01001` 500（`internal/pkg/response/response.go:37-38`）。Postgres 中唯一冲突会使事务进入 aborted 状态，因此"兜底"不可能被上层优雅处理，只能整笔失败。
- 建议修复：① 幂等键加命名空间前缀校验（充值键禁止以 `order:` 开头）或把唯一索引改为 `(user_id, idempotency_key)` + 订单键使用不可被用户写入的保留空间；② 把 `repoErrWalletLogAlreadyExists` 导出并在 `PayOrder` 中映射为明确的业务错误（或按订单当前状态重新判定后幂等返回）；③ `DepositDTO.IdempotencyKey` 增加 `max=64` 与字符集约束。
- 影响范围：单个订单永久不可支付（500），超时后由关单流程兜底；不会造成资金损失，但会把"防御性幂等"变成不可恢复的故障，且触发方可以是普通买家（自伤）或一次数据修复/导入。

### P2（8 条）

**P2-1 超时关单不校验 `payment_deadline`，只隐式信任 MQ 延迟时序**

- 现象：`HandleOrderTimeout` 锁定订单后仅判断 `status == processing` 就关单并回补库存/优惠券（`internal/order/repository.go:159-183`），完全不比对 `payment_deadline`。当前 `payment_deadline = now + mqCfg.TTLMs`（`internal/order/service.go:129`）与队列 `x-message-ttl` 同源（`internal/order/repository.go:59-63`），所以正常运行不会误关；但一旦通过 RabbitMQ **policy** 改 TTL、人工重投超时消息、或未来新增"手动关单/补发消息"入口，就会把**仍可支付**的订单提前关闭并释放库存与优惠券，买家随后只能得到 `A05102`。
- 代码位置：`internal/order/repository.go:159-183`；`internal/order/service.go:198-226`（先关单再还原库存/券）；`internal/order/mq_handler.go:85-115`（4 次重试，重投不加时间约束）。
- 依据：代码路径 + `docs/order-payment.md:30`（"关闭订单由现有超时流程负责"但未声明时序约束）。
- 建议修复：在 `HandleOrderTimeout` 内加 `if time.Now().Before(o.PaymentDeadline) { return nil, nil }`（并且仅当状态与截止时间同时满足才关单），使截止时间成为唯一事实来源，MQ 只负责"触发"。
- 影响范围：业务规则违背（提前关单、库存/券被过早释放）；无资金损失。

**P2-2 金额用 `float64` 承载 `numeric(16,2)`，比较依赖 numeric→float8 隐式转换**

- 现象：`UserWallet.Balance` / `WalletLog.Amount` / `Order.PayableAmount` 均为 `float64`，列为 `numeric(16,2)`；扣款条件 `WHERE balance >= ?` 与 `user_wallets.balance - ?` 中参数是 float8，Postgres 会把 numeric 侧隐式提升为 float8 再比较/运算（`internal/wallet/repository.go:97-102`）。在 2 位小数、金额 < 2^53 分的现实区间内比较是精确的（两侧都是同一小数的最短表示舍入），但列类型允许到 10^14 量级，超过 2^53 分后会出现分位漂移；同时该写法无法使用 `balance` 上的任何索引（PK 命中，暂无影响）。
- 代码位置：`internal/model/wallet.go:21-40`；`internal/model/order.go:35-55`；`internal/wallet/repository.go:97-102,128-133`；`migrations/20260507035237.sql:49-54`。
- 依据：列定义 + 表达式写法；`docs/order-payment.md:9` 声明的"条件更新"本身是原子的（这一点无问题）。
- 建议修复：金额统一改为 `int64`（分）或 `decimal`/字符串类型，SQL 侧显式 `$1::numeric`；至少把 `float64` 参数显式转换成 numeric 以免隐式提升。
- 影响范围：理论精度风险与长期可维护性；当前不会造成实际账目偏差（金额恒 2 位小数且买卖两侧等额）。

**P2-3 支付路径的业务错误未映射，统一退化为 `B01001`；扣款 0 行一律解释为"余额不足"**

- 现象：`PayOrder` 只把 `ErrInsufficientBalance` / `ErrOrderPaymentExpired` / `ErrOrderPaymentStatusInvalid` / `NotFound` 映射成业务码（`internal/order/service.go:287-299`）。`CreditOrderIncome` 的 "seller wallet not found"（`internal/wallet/repository.go:137-140`）、`createLog` 的任何非唯一冲突错误、以及 `DebitForOrderPayment` 的包装错误都会变成未分类错误 → `B01001 系统繁忙`，买家重试永远无意义。另外钱包行**缺失**时扣款 `RowsAffected == 0` 被当成 `A03102 余额不足`（`internal/wallet/repository.go:106-108`），会把数据异常伪装成"钱不够"。
- 代码位置：`internal/order/service.go:287-299`；`internal/wallet/repository.go:103-108,134-140`；`internal/pkg/response/response.go:37-38`。
- 依据：错误传播路径；`tests/order_payment_test.go:161-174` 只覆盖真实余额不足。
- 建议修复：为"钱包/卖家账户缺失"定义独立业务码（如 B 类但带明确 message 与告警），扣款侧区分 `balance < amount`（先查余额或使用 `RETURNING`）与"钱包不存在"。
- 影响范围：可观测性与排障；用户侧的错误提示不准确。

**P2-4 账本可追溯性不足：商家入账流水复用买家 `session_id`，`wallet_logs` 无 `user_id` 索引/无 `order_id` 列**

- 现象：`CreditOrderIncome` 的 `session_id` 传的是**买家**的会话（`internal/order/service.go:273` → `internal/wallet/repository.go:117-123`），"哪个会话导致了商家余额变化"会指向买家会话；`wallet_logs` 只有 `id` 主键与全局唯一幂等键，没有 `(user_id, created_at)` 索引，也没有 `order_id` 列（`migrations/20260507035237.sql:56-65`），按用户查流水是全表扫描，按订单对账只能对字符串键做 `LIKE`。
- 代码位置：`internal/wallet/repository.go:113-142,158-165`；`migrations/20260507035237.sql:56-65`。
- 依据：表定义与写入参数来源。
- 建议修复：为入账流水写系统/订单维度标识（如 `session_id = "order:<id>"` 或新增 `order_id` 列）；补 `(user_id, created_at)` 与 `(type, created_at)` 索引。
- 影响范围：对账、风控、审计取证效率；无功能影响。

**P2-5 零元订单不产生任何流水，"零元已支付"与"流水缺失"无法区分**

- 现象：`PayableAmount <= 0` 时跳过扣款与入账（`internal/order/service.go:266-276`），订单直接置 4，不写任何 `wallet_logs`（`tests/order_payment_test.go:259-287` 明确了该行为）。与文档一致（`docs/order-payment.md:41`），但对账时"零元已支付订单没有流水"与"流水写失败/被删"在数据上不可区分。
- 代码位置：`internal/order/service.go:266-276`；`internal/wallet/repository.go:83-85,114-116`（`amount <= 0` 直接返回）。
- 依据：代码路径 + 上述用例。
- 建议修复：零元订单写一条 `amount = 0` 的 `order_payment`/`order_income` 标记流水（或 `type=order_free`），保持"每个已支付订单至少有账本记录"的不变式。
- 影响范围：对账完整性；无资金影响。

**P2-6 契约文档缺口：OpenAPI 未声明"仅 buyer 可调用"，`updated_at` 的 `format: date-time` 与实际返回格式不符**

- 现象：① 路由确实只有买家可用（`internal/app/bootstrap.go:161` + `internal/middleware/auth.go:72-81`，非买家得到 `A02100`），但 OpenAPI 的 `/order/{id}/pay` 只写了 `AccessTokenAuth`（`internal/app/openapi.yaml:421-447`），未说明角色要求，也未在错误清单里列 `A02100`；`docs/order-payment.md:23-28` 同样缺少该行。② 订单详情/支付响应中的 `updated_at` 在 spec 里声明 `type: string, format: date-time`（`internal/app/openapi.yaml:985-987`），实际输出是 `"2006-01-02 15:04:05"`（无时区、非 RFC3339，`internal/order/response.go:74`），严格客户端解析会失败；`created_at` 同样是无时区字符串，但因 spec 未声明 format 而"恰好合规"。
- 代码位置：`internal/app/openapi.yaml:421-447,969-987`；`internal/order/response.go:41-77`；`internal/app/bootstrap.go:161`；`internal/middleware/auth.go:72-81`。
- 依据：路由/中间件代码 + 响应格式化代码与 spec 对照。
- 建议修复：在 pay 描述的 security/description 中补 "仅买家"、错误清单补 `A02100`；`updated_at`/`created_at` 改为 RFC3339（或把 spec 的 format 去掉）。
- 影响范围：对外契约稳定性、SDK 生成与前端解析。

**P2-7 支付可观测性不足：重复支付计入 success，退款占位返回系统故障码**

- 现象：① `AddOrderPaymentTotal` 在 `err == nil` 时一律记 `status=success, code=none`（`internal/order/service.go:238-246`、`internal/order/metrics.go:104-108`），而查表幂等返回（订单已支付）同样走该分支，指标无法区分"真实扣款笔数"与"重复请求"，也无法用 `order_payment_total{code}` 做资金流量监控。② `POST /order/:id/refund` 目前对合法请求返回 `B01001`（`internal/order/handler.go:166-191`），会被当成系统故障计入告警。
- 代码位置：`internal/order/service.go:238-246`；`internal/order/metrics.go:42-51,104-108`；`internal/order/handler.go:166-191`。
- 依据：指标埋点路径 + 占位处理器。
- 建议修复：新增 `already_paid` 结果维度（或在判定重复支付时单独计数）；退款占位返回明确的 `A` 类"未实现"码。
- 影响范围：监控/告警质量；不影响交易正确性。

**P2-8 滚动发布顺序：本次支付依赖的 `NOT NULL` 列无数据库默认值，旧代码写入会 500**

- 现象：`orders.payable_amount` / `payment_deadline` 是 `NOT NULL` 且**无 DEFAULT**（`migrations/20261031000300_order_payment_fields.sql:1-16`）。若先跑迁移再滚动发布，旧版本 Pod 的 `INSERT orders`（不带这两列）会直接失败 → 发布窗口内下单 500；反之先发新代码则列不存在。需要严格按 expand-contract 两阶段（先加可空列 + 回填 + 加 NOT NULL，再切代码）执行。
- 代码位置：`migrations/20261031000300_order_payment_fields.sql:1-16`；`internal/order/service.go:126-143`（新代码显式赋值）。
- 依据：迁移 DDL 与写入列对照；迁移已在 `migrations/atlas.sum` 中登记（不会因校验和阻塞）。
- 建议修复：发布流程显式拆分"迁移 → 回填 → 加约束"与"代码切换"两步，或临时给列加 DEFAULT。
- 影响范围：部署期可用性；不影响已上线后的正确性。

## 已核对无问题的点

**并发与锁**

1. **锁顺序一致**：支付先锁 `orders` 行（`internal/order/repository.go:129-141`，`FOR UPDATE` + `user_id` 限定），超时关单同样先锁 `orders` 行（`internal/order/repository.go:160-164`）；钱包行只出现在支付路径内部，且固定为"买家行 → 商家行"（`internal/order/service.go:267,273`）。没有任何事务先拿钱包行再拿订单行，因此**不存在锁顺序反转，也就没有死锁环**。
2. **交叉死锁被账号模型排除**：`user_type` 是互斥枚举（`internal/model/user.go:19-25`），买家账号不可能同时是商家；"A 买 B 的商品、B 买 A 的商品"这种可能产生 A↔B 钱包互等的形态在现网不可达。**若未来允许一个账号既买又卖，必须改成按 `user_id` 排序加锁**（前瞻性约束，非当前缺陷）。
3. **并发支付同一订单不重复扣款**：订单行锁串行化；READ COMMITTED 下等待方在获得锁后读到的是已提交的最新版本，看到 `status=4` 即走幂等返回（`internal/order/service.go:253-257`），`MarkOrderPaid` 还有 `status=0` 的条件更新兜底（`internal/order/repository.go:144-155`）。对应用例 `tests/order_payment_test.go:220-245`（12 并发、断言余额只扣一次、流水仅 2 条）；本机无 Docker 未执行，结论由代码路径推理。
4. **支付与超时关单双向互斥**：两者以同一行锁为线性化点。支付先提交 → 超时看到非 processing 直接返回 nil，不重复释放库存/归还券、不覆盖 `paid_at`（`internal/order/repository.go:165-181`、`internal/order/service.go:209-224`，用例 `tests/order_payment_test.go:247-257`）；超时先提交 → 支付看到状态非 0 返回 `A05102`（用例 `tests/order_payment_test.go:190-197`）。
5. **无丢失更新窗口**："判断余额是否足够 + 扣减"是单条 SQL 的原子条件更新（`internal/wallet/repository.go:97-102`），入账用 SQL 表达式累加（`internal/wallet/repository.go:128-133`），不存在应用层读-改-写。
6. **事务原子性与连接一致性**：`ExecuteTransaction` 把 `*gorm.DB` 放进 context（`internal/pkg/database/transaction.go:15-20`），`BaseRepo.GetDB` 从 context 取事务（`internal/pkg/database/base_repo.go:17-23`），订单锁、买家扣款、商家入账、订单置已支付、回读全部落在同一事务同一连接；任一步失败整体回滚（`tests/order_payment_test.go:161-174` 验证余额不足时连已写入的流水也回滚）。提交前进程崩溃 → 回滚；提交后响应丢失 → 重发走幂等返回，无"钱扣了订单没变"窗口。

**金额与账本**

7. **买卖两侧恒等**：扣款与入账使用同一个 `current.PayableAmount`（`internal/order/service.go:266-275`），金额符号相反、数量相等，且在同一事务内提交；零元订单两侧都不动（`tests/order_payment_test.go:259-287`）。`balance` 与 `Σ wallet_logs` 在每个事务边界上保持一致（没有事务外改余额的路径）。
8. **`payable_amount` 快照语义正确**：下单时按 `round(单价×数量,2) - round(折扣,2)` 取整到分并落库（`internal/order/service.go:125,128,304-307`），支付时只读该快照、不重读商品价格（`internal/order/service.go:267,273`）；响应 `total_amount` 即 `payable_amount`（`internal/order/response.go:48-63`）。
9. **历史订单无"NULL 当 0 白拿"窗口**：迁移先加可空列 → 用 `snapshot_price×quantity-discount_amount` 回填 → `SET NOT NULL` 并加 `CHECK (payable_amount >= 0)`，同时回填 `payment_deadline`（`migrations/20261031000300_order_payment_fields.sql:1-16`）。旧订单都有确定金额与截止时间。
10. **幂等键命名规则一致**：`order:<id>:payment` / `order:<id>:income` 由同一处生成（`internal/wallet/repository.go:158-165`），不随用户/会话变化；两者后缀不同，即使出现"自己买自己"也不会互相冲突。
11. **卖家钱包存在性假设成立**：注册事务内即创建钱包（`internal/user/service.go:65-90`，`CreateDefaultAccount` 在 86-88 行），并对存量用户做了回填（`migrations/20261101000200_backfill_account_wallets.sql`）；缺失时整笔支付回滚（`internal/wallet/repository.go:137-140`），不会出现"买家已扣、商家未收"的单边账。

**契约与安全**

12. **仅买家可支付**：路由挂了 `RequireUserType(model.UserTypeBuyer)`（`internal/app/bootstrap.go:161`），且该中间件同时校验 `status == active`（`internal/middleware/auth.go:75`），pending/disabled 账号同样被拒（`A02100`）。
13. **他人订单返回 `A00002` 且不泄露存在性**：锁查询带 `id = ? AND user_id = ?`（`internal/order/repository.go:131-133`），找不到统一映射为 `ErrNotFoundRecord`（`A00002`），与"订单不存在"不可区分；用例 `tests/order_payment_test.go:199-218` 断言对方余额不变。
14. **错误码与 OpenAPI/文档三方一致**：`A00002` / `A03102` / `A05102` / `A05103` 与 `pkg/errno/code.go:20-31`、`internal/app/openapi.yaml:439-443`、`docs/order-payment.md:23-28` 完全对应；成功码 `000000` 与 `errno.OK.FullCode()` 一致（顺带说明：其他旧接口文档写的 `00000` 是历史笔误，新接口写对了）。
15. **响应不泄漏非买家信息**：支付响应复用 `OrderDetailResponse`（`internal/order/response.go:41-77`），只含订单快照与状态字段（含 `seller_id` 这类订单本身已包含的标识），**不含任何钱包/商家余额**；余额只能通过 `GET /wallet/balance` 查自己（`internal/wallet/handler.go:60-70`）。`APP_APP_EXPOSE_DEV_MSG=false` 在配置文件与 k8s ConfigMap 中均为关闭（`configs/config.yaml:11`、`k8s/app/configmap.yaml:16`），不会回传 SQL/原始错误。
16. **重复支付的状态码语义合理**：返回 `200 + 000000` + 订单详情（含 `paid_at`），客户端可据 `paid_at`/`status` 判断"此前已支付"，与 `docs/order-payment.md:15-19` 一致；如需显式区分，建议在 `data` 中加 `already_paid` 标志（非缺陷）。
17. **路由无冲突**：`GET /order/list` 与 `GET /order/:id`、`POST /order/create` 与 `POST /order/:id/pay` 在 gin v1.11 下静态段与参数段可并存（`internal/app/bootstrap.go:157-162`），不会启动 panic；`/order/create` 仍优先命中静态路由。

## 验证记录

- `go build ./...` → 通过；`go vet ./...` → 通过；`go test ./internal/order -count=1` → 通过（handler/mq 单测，不依赖外部服务）。
- **未执行** `tests/` 下的 Testcontainers 用例（本机无 Docker，避免无效超时）：`tests/order_payment_test.go` 的全部用例、`tests/order_invariants_test.go` 的详情/越权用例。文中引用它们时均标注为"用例存在、本机未执行"。
- 未连接、未改动 `192.168.1.106` 上的任何服务；未 commit、未 push；本次唯一写入文件为 `docs/audit/payment-review.md`。
- 可复现的遗留验证建议（需 Docker/预发环境）：① 并发支付同一订单（对应用例已存在）；② 支付与超时关单同时触发；③ 用充值接口预占 `order:<id>:payment` 后支付（验证 P1-1）；④ 同一买家两笔订单并发支付验证不超扣。
