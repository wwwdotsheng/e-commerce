# 日志与错误码的监控

## 前提：这个应用统一返回 HTTP 200

e-commerce 的约定是 **HTTP 状态码恒为 200，用响应体里的业务码区分结果**：

```json
{ "code": "A02103", "data": null, "userMsg": "非法访问" }
```

所以：

```
按 HTTP status 建监控  →  什么都看不到（全是 200）
按业务码 code 建监控   →  信号全在这里  ✅
```

面板 `e-commerce 日志与错误码` 就是按 `code` 建的。

## ★ 错误码分两类，监控必须分开看

代码定义在 `pkg/errno/code.go`，前缀区分性质：

### A 开头 —— 业务/认证类（用户操作导致，属正常）

| 码 | 含义 | 备注 |
|---|---|---|
| A00001 | 提交参数非法 | 客户端 bug 或用户乱填 |
| A00002 | 记录不存在 | |
| A01102 | 用户名已存在 / 邮箱已被注册 | |
| A01103 | 账号或密码错误 | **持续高位 = 可能被撞库** |
| A02100 | 无权限访问 | |
| A02101 | 令牌已过期 | 正常，用户重新登录即可 |
| A02102 | 账号已在别处登录 | 正常业务 |
| A02103 | 非法访问（token 无效） | **抬头 = 可能有人在伪造 token** |
| A03101 | 充值金额非法 | |
| A04101 | 库存不足 | 正常抢购场景 |
| A04102 | 商品不存在 | |
| A04103 | 商品状态参数无效 | |
| A05100 | 商品 ID 不存在 | |

### B 开头 —— 系统类（真正的服务故障，该告警的是它）

| 码 | 含义 |
|---|---|
| B01001 | 系统繁忙，请稍后重试 |
| B01002 | 数据库操作异常 |
| B01003 | 无法获取 accountInfo |
| B03001 | 缓存服务暂时不可用 |

**A 码涨了不一定要管，B 码涨了必须管。** 混在一起看，告警会被用户输入错误淹没。

## 面板位置

Grafana（`monitoring` namespace）→ 面板 `e-commerce 日志与错误码`

```
① 系统错误（B 开头）          这个必须为 0
② 认证失败（A02103）          可疑时看这里
③ 登录失败（A01103）          可能是撞库
④ 总请求数
⑤ 系统错误趋势（按 code）
⑥ 认证类错误趋势（A021xx）
⑦ 请求量（按接口）
⑧ 业务失败排行（按 code）
⑨ HTTP 状态码（对照用）
```

## 日志是怎么到 Loki 的

```
应用 → stdout（JSON 格式）
  ↓  containerd 写日志文件
Alloy DaemonSet（4 个节点全覆盖）
  ↓  采集容器日志，打上 namespace/pod/container 标签
Loki（observability namespace）
  ↓  LogQL 查询
Grafana（monitoring namespace，数据源由 overlay 注入）
```

**应用必须把日志写 stdout，不能写文件** —— 这是 `pkg/clog` 改成文件输出可选的原因。

## 关键日志字段

```json
{
  "level": "info",
  "ts": "2026-10-11T03:34:47+0800",
  "caller": "response/response.go:54",
  "msg": "business_warning",        ← A 码用这个；B 码是 "system_fault"
  "trace_id": "...",                 ← 可以串起一次请求的全部日志
  "span_id": "...",
  "code": "A02103",                  ← 业务码
  "raw": "token is malformed: ..."   ← 具体原因
}
```

**常用 LogQL：**

```logql
# 所有系统错误
{namespace="ecommerce"} | json | msg="system_fault"

# 按业务码过滤
{namespace="ecommerce"} | json | code="A02103"

# 按业务码统计
sum by (code) (count_over_time({namespace="ecommerce"} | json | msg=~"business_warning|system_fault" [5m]))

# 用 trace_id 追一次请求
{namespace="ecommerce"} |= "<trace_id>"
```
