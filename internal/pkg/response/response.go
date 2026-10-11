package response

import (
	"e-commerce/internal/pkg/contextx"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-faster/errors"
	"go.uber.org/zap"
)

// WriteInvalidParam 参数校验失败快捷方式，自动包装原始错误便于调试
func WriteInvalidParam(c *gin.Context, err error) {
	Write(c, errno.ErrInvalidParam.WithRaw(err), nil)
}

func Write(c *gin.Context, err error, data interface{}) {
	ctx := c.Request.Context()
	logger := clog.L(ctx)

	cfg := contextx.GetConfig(c)

	resp := gin.H{
		"code":    errno.OK.FullCode(),
		"userMsg": errno.OK.Message,
		"data":    data,
	}

	if err == nil {
		c.JSON(http.StatusOK, resp)
		return
	}

	var e *errno.Errno
	if !errors.As(err, &e) {
		e = errno.ErrInternalServer.WithRaw(err)
	}

	rawMsg := ""
	if e.RawErr != nil {
		rawMsg = e.RawErr.Error()
	}

	displayMsg := e.Message
	if e.Type == "B" || e.Type == "C" {
		logger.Error("system_fault", zap.String("code", e.FullCode()), zap.Error(e.RawErr))
		displayMsg = errno.ErrInternalServer.Message
	} else {
		logger.Info("business_warning", zap.String("code", e.FullCode()), zap.String("raw", rawMsg))
	}

	resp["code"] = e.FullCode()
	resp["userMsg"] = displayMsg
	resp["data"] = nil

	// devMsg 会带上原始错误（含 SQL、连接串等），默认不返回。
	// 排查问题时用 APP_APP_EXPOSE_DEV_MSG=true 临时打开。
	if cfg.App.ExposeDevMsg {
		resp["devMsg"] = e.Error()
	}

	c.JSON(http.StatusOK, resp)
}
