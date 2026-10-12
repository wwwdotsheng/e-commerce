# Secret 管理与 GitOps

## 一、GitOps 的两条判据

比定义好用：

```
判据 1：Git 是 single source of truth
        集群里的一切都能从 Git 复现，不需要额外的手工步骤

判据 2：有 controller 持续 reconcile
        Git 变了 → 自动 sync
        有人手改了集群 → 拉回 desired state

两条都满足 = GitOps
只满足判据 1 的一半 = declarative deployment（比 kubectl run 强，但不是 GitOps）
```

## 二、Secret 与 GitOps 是根本冲突

```
GitOps 要求：所有 desired state 都在 Git 里
安全要求：   Secret 不能在 Git 里（明文）

→ 这不是配置问题，是两个正确要求的直接冲突
→ 所有 Secret 管理方案，都是对这个冲突的不同回答
```

冲突无法「解决」，只能选择在哪一侧让步：

```
让 Git 让步   → 承认 Git 不是 single source of truth，Secret 单独管理
让安全让步    → Secret 明文进 Git（不可接受）
第三条路      → 让 Secret 这个概念变形：
                ① 以密文形式进 Git
                ② 以引用形式进 Git
```

## 三、三条路径

### 路径 ①：密文进 Git

```
Sealed Secrets
  用集群公钥加密 → SealedSecret 可以进 Git
  apply 一个 SealedSecret → controller 用私钥解成 Secret
  ✅ 满足两条判据
  ❌ 密钥轮换麻烦；绑集群（换集群要重新加密）

SOPS + age/KMS
  加密 YAML 文件本身，解密在 CI 或集群侧
  ✅ 同样满足
  ❌ 要管理解密密钥的分发
```

### 路径 ②：引用进 Git

```
External Secrets Operator
  Git 里放 ExternalSecret：
    { 从 Vault 的 secret/data/ecommerce 拿 AUTH_TOKEN_SECRET }
  ESO 从 Vault / AWS Secrets Manager 同步成 K8s Secret
  ✅ 满足两条判据，而且能集中管理和轮换
  ❌ 要多维护 Vault / 外部系统

Vault Agent Injector
  sidecar 直接把值注入容器文件系统，连 Secret 对象都不产生
  ✅ 最彻底
  ❌ 侵入性最强
```

### 路径 ③：承认 Git 不是 single source of truth

```
手工 kubectl create secret
或 Secret 清单做模板 + 本地真值文件       ← ★ 本项目现在这里

✅ 简单，零额外组件
❌ 不满足判据 1 —— 从 Git 复现不出完整状态
```

## 四、本项目在哪一层

**已经做到的：**

```
· 51 个清单文件在 Git 里
· ecommerce namespace 21 个资源 + observability namespace 11 个资源，形状都在清单里
· Secret 的名字和 key 名也在清单里（k8s/*/secret.yaml 是模板）
· Secret 的值不在 Git：本地 secret.local.yaml，已加进 .gitignore
· kustomization 刻意不引用 secret.yaml —— kubectl apply -k 不会碰 Secret
```

**缺的：**

```
· 没有 GitOps controller
  argocd / flux-system namespace 都不存在
  （只有 applications.argoproj.io / appprojects.argoproj.io 两个 CRD，
    2026-07-03 装的，没有 controller）
  → 判据 2 完全没有

· 4 个 Secret 的值在本地 secret.local.yaml，不在 Git
  → 判据 1 差一点

· 部署靠手工 kubectl apply -k（CI 里连 deploy job 都没有）
```

**结论：declarative manifests + 手工 apply + 本地 Secret，即路径 ③。**

## 五、★ Secret 方案是 GitOps 的前置条件

不是「顺便处理一下」，是**前置**：

```
上 Argo CD 时会遇到：
  sync 清单 → Deployment 引用了 ecommerce-app-secret
            → 但 Secret 不存在（真值在本地 secret.local.yaml，Argo CD 看不到）
            → Pod 起不来
            → Application 一直 OutOfSync / Degraded

→ 必须先决定 Secret 走哪条路径，才能上 GitOps
→ 顺序反了会卡住
```

## 六、升级路径

| 阶段 | 做法 | 判据 1 | 判据 2 | 成本 |
|---|---|---|---|---|
| **0（现在）** | `secret.yaml` 模板 + 本地 `secret.local.yaml` + 手工 apply | 差一点 | ✗ | 0 |
| **1** | CI 里从变量生成 `secret.local.yaml` 再 apply | 差一点 | ✗ | CI 加几行 |
| **2** | Argo CD + 选一条路径（①或②） | ✓ | ✓ | 一个组件 + 选型 |

**阶段 1 和 2 之间没有中间态** —— 上 Argo CD 就必须先解决 Secret。

---

# 操作手册

## 目录结构

```
k8s/app/
├── secret.yaml           ← 提交进 Git：模板，只有占位符
├── secret.local.yaml     ← 不提交：真实值（在 .gitignore）
├── kustomization.yaml    ← ★ resources 里【不含】secret.yaml
└── ...
```

## 为什么 kustomization 不引用 secret.yaml

```
如果引用了：
  kubectl apply -k → 把集群里的真值覆盖成模板里的占位符 ✗

不引用：
  kubectl apply -k → 完全不碰 Secret ✓
  代价：Secret 要单独应用（scripts/apply-secrets.sh 干这事）

已验证：构建产物里 4 个 Secret 全部被排除。
```

## 首次部署

```bash
# 1. 从模板复制出本地真值文件
for m in app postgres redis rabbitmq; do
  cp k8s/$m/secret.yaml k8s/$m/secret.local.yaml
done

# 2. 填真实值
openssl rand -base64 48 | tr -d '\n'     # 填进 app 的 AUTH_TOKEN_SECRET
# postgres / redis / rabbitmq 的密码也要改

# 3. 应用 Secret（单独一步）
./scripts/apply-secrets.sh

# 4. 应用其余资源
kubectl apply -k k8s/overlays/harbor-local
```

## ★ 改 Secret 的规则

```
改 secret.local.yaml，然后重跑 ./scripts/apply-secrets.sh

不要直接 kubectl edit secret —— 下次跑脚本会被 .local.yaml 覆盖回去
```

## 必需的 Secret（5 个 key）

```
ecommerce-app-secret.AUTH_TOKEN_SECRET          ← Deployment/ecommerce-api
postgres-secret.POSTGRES_PASSWORD               ← Deployment/ecommerce-api
rabbitmq-secret.RABBITMQ_DEFAULT_PASS           ← Deployment/ecommerce-api
redis-secret.REDIS_PASSWORD                     ← Deployment/ecommerce-api
redis-secret.REDIS_PASSWORD                     ← StatefulSet/redis
```

## ⚠️ 这个方案的已知代价

```
如果部署者忘了跑 apply-secrets.sh 就 apply -k：
  → Deployment 引用了不存在的 Secret
  → Pod 起不来，报错只说 "secret not found"
  → 没有前置校验，要等 Pod 起不来才发现

缓解：apply-secrets.sh 会检查 .local.yaml 是否存在、是否还是占位符
```

## 换密码的代价

| 组件 | 代价 |
|---|---|
| **app / AUTH_TOKEN_SECRET** | 只影响 JWT 签名。改完 restart 即可，**旧 token 全部失效，用户需重新登录** |
| **postgres** | 密码在 `initdb` 时写进数据目录。改 Secret 不会改库里的密码 —— 必须同时执行 `ALTER USER postgres WITH PASSWORD '...'`，否则 app 用新密码连不上旧密码的库。**需要停机窗口** |
| **redis** | 要同步改 `requirepass` 并 restart |
| **rabbitmq** | 要同步 `rabbitmqctl change_password` 并 restart |

**换 app 之外的密码，不要在业务高峰期做。**
