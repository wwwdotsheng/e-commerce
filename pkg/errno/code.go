package errno

var (
	OK = &Errno{Type: "0", Domain: "00", Code: "000", Message: "OK"}

	ErrInvalidParam   = &Errno{Type: "A", Domain: "00", Code: "001", Message: "提交参数非法"}
	ErrNotFoundRecord = &Errno{Type: "A", Domain: "00", Code: "002", Message: "记录不存在"}

	ErrUserNameExisted  = &Errno{Type: "A", Domain: "01", Code: "102", Message: "用户名已存在"}
	ErrUserEmailExisted = &Errno{Type: "A", Domain: "01", Code: "102", Message: "邮箱已被注册"}
	ErrUserNotFound     = &Errno{Type: "A", Domain: "01", Code: "103", Message: "账号或密码错误"}

	ErrAuthNotPermission  = &Errno{Type: "A", Domain: "02", Code: "100", Message: "无权限访问"}
	ErrAuthTokenExpired   = &Errno{Type: "A", Domain: "02", Code: "101", Message: "令牌已过期"}
	ErrAuthSessionRevoked = &Errno{Type: "A", Domain: "02", Code: "102", Message: "账号已在别处登录"}
	ErrAuthInvalidToken   = &Errno{Type: "A", Domain: "02", Code: "103", Message: "非法访问"}

	ErrWalletInvalidDepositAmount = &Errno{Type: "A", Domain: "03", Code: "101", Message: "充值金额非法"}

	ErrProductStockInsufficient = &Errno{Type: "A", Domain: "04", Code: "101", Message: "库存不足"}
	ErrProductNotFound          = &Errno{Type: "A", Domain: "04", Code: "102", Message: "商品不存在"}
	ErrProductStatusInvalid     = &Errno{Type: "A", Domain: "04", Code: "103", Message: "商品状态参数无效"}

	// ErrOrderProductIdNotFound 下单时输入的商品 ID 在系统中无法找到
	ErrOrderProductIdNotFound = &Errno{Type: "A", Domain: "05", Code: "100", Message: "商品ID不存在"}

	ErrInternalServer = &Errno{Type: "B", Domain: "01", Code: "001", Message: "系统繁忙，请稍后重试"}
	ErrDatabase       = &Errno{Type: "B", Domain: "01", Code: "002", Message: "数据库操作异常"}
	ErrGetAccountInfo = &Errno{Type: "B", Domain: "01", Code: "003", Message: "无法获取accountInfo信息"}

	ErrRedisDown = &Errno{Type: "C", Domain: "03", Code: "001", Message: "缓存服务暂时不可用"}
)
