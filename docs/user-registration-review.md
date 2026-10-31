# 用户注册与卖家审核

- 买家注册：`POST /api/v1/user/register/buyer`，Body：`user_name`、`email`、`password`。
- 卖家注册：`POST /api/v1/user/register/seller`，另需 `shop_name`（最多128字符）、`application_note`（最多1000字符）。
- 登录入口不变。`GET /api/v1/user/me` 查看类型、状态及审核原因，不返回密码。
- 被拒绝后：`PATCH /api/v1/user/seller-application`，Body 为修改后的 `shop_name` 和 `application_note`，重新进入 pending。
- 管理员列表：`GET /api/v1/admin/seller-applications?status=pending&page_num=1&page_size=10`，单页最多50条。
- 管理员详情：`GET /api/v1/admin/seller-applications/:id`。
- 审核：`POST /api/v1/admin/seller-applications/:id/review`，Body：`{"decision":"approve","reason":""}` 或 `{"decision":"reject","reason":"拒绝原因"}`。拒绝原因必填，最多1000字符。

类型由服务端设置，公开接口不能创建管理员。邮箱和用户名沿用全局唯一约束。待审核/被拒绝的卖家仅允许访问自身资料、重新提交及认证接口；开店及商品写接口要求 active seller。买家负责下单和申请退款。审核使用事务、行锁及审核日志，重复处理返回 A01104。

应用迁移后，既有用户默认 buyer/active，已有店铺或发布商品的用户迁移为 seller/active。管理员由运维在数据库中将指定已有账号的 user_type 设为 admin，不提供默认管理员账号或公开提权接口。鉴权每次读取当前用户类型和状态，禁用账号不能登录，使用旧令牌访问时该会话会被撤销。

原 `/user/register` 路由替换为上述两个入口，调用方应更新。Kubernetes 业务验收测试增加 `API_TEST_ADMIN_TOKEN` 参数，用于审核该次测试新建的卖家账号。

本次未修改 OpenAPI 草稿，审核接口以后续同步的文档为准。
