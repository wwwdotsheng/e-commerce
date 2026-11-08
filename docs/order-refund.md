# 订单退款（异步受理，一期）

一期实现“受理 + 后台处理 + 可查询”的退款流程：

- 用户申请通过校验后立即受理，无需商家审批；
- 仅支持全额退款，金额取订单 `payable_amount` 快照；
- **HTTP 请求内不调用第三方、不重试、不启动 goroutine**；
- 实际渠道调用由未来的后台任务执行器完成，本阶段尚未接入；
- 因此申请成功后状态为 `pending`，不会自动变成 `succeeded`；这是预期行为。

退款成功会退回本次使用的优惠券（已过期则标记 expired）；本期不做站内钱包反向转账、库存返还、部分退款、真实支付渠道。

## 两张表

### refunds

退款业务身份与当前/最终状态，一个订单最多一条记录（`order_id` 唯一）。

- `id`：稳定 `refund_id`，后续调用/查询第三方都用它，重试不重新生成；
- `order_id`、`buyer_id`、`amount`、`status`、`failure_reason`、`refunded_at`；
- 成功后 `refunded_at` 写入；失败/未完成保持 null。

### refund_jobs

待执行任务，进程重启后仍然存在。同一退款最多一条活动任务（`refund_id` 唯一）。

- `job_type`：
  - `create_refund`：发起退款（首次受理，或查询确认上次未处理后再发起）；
  - `query_refund`：结果不明时按同一 `refund_id` 查询确认；
- `next_run_at`：任务到期时间，worker 按它取任务；
- `create_attempts` / `query_attempts`：分别统计发起/查询次数，查询不消耗退款重试额度；
- `last_error`：最近一次执行失败摘要，供排查；完整信息仍写日志；
- 成功/失败终态落库后删除任务，`refunds` 永远保留。

## 状态

```
pending ──worker领取──> processing ──成功──> succeeded（终态）
                             │
                             ├──可重试失败──> wait_retry ──> pending/processing
                             ├──不可重试/次数耗尽──> failed（终态）
                             ├──查询次数耗尽──> manual（终态，待人工处理）
                             └──结果不明──> query_refund ──> succeeded / failed / manual / create_refund
```

- `pending` / `processing` / `wait_retry`：未终态，前端显示“处理中/等待重试”；
- `succeeded` / `failed` / `manual`：终态；`manual` 表示查询次数耗尽、需要人工处理；
- 只有 `succeeded` 才把订单从 `4 已支付` 改为 `5 已退款`；
- 订单在 `pending/processing/wait_retry/failed` 时保持 `4 已支付`。

## 接口

| 接口 | 行为 |
|---|---|
| `POST /api/v1/order/{id}/refund` | 校验买家/归属/已支付，事务内写 refunds + refund_jobs，立即返回 `pending`；重复/并发申请返回同一条记录 |
| `GET /api/v1/order/{id}` | 订单详情；`refund` 只读返回当前退款状态，`timeline` 返回订单+退款的状态时间线，无退款时 refund 为 null |

错误码：

- `A00001` 参数非法
- `A00002` 订单/退款不存在或不属于当前用户
- `A02100` 非买家
- `A05105` 订单当前状态不可退款

## 后台执行器

- 每 5 秒扫描 `refund_jobs.next_run_at <= now()`，单批 20 条；
- 调用前短事务记录尝试次数并设置 `next_run_at = now + 30s`；
- 外部调用在事务外；结果落库时锁 `refunds` 行，同一事务更新订单状态、任务；
- `create_refund` 结果不明 → 切 `query_refund`；查询“查无此退款” → 切回 `create_refund`；
- create 最多 5 次、query 最多 10 次；失败按指数退避 5s、10s、20s… 最大 10 分钟；
- query 次数耗尽仍不明 → 退款置 `manual`、删除任务，等待人工处理（当前无后台放行入口）；
- 崩溃恢复：`create_refund` 且退款为 `processing` 时，先查询渠道，不直接重新发起。
