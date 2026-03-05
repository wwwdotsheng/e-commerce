package res

import (
	"e-commerce/internal/config"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

func WriteResponse(c *gin.Context, err error, data interface{}) {
	ctx := c.Request.Context()
	logger := clog.L(ctx)

	resp := gin.H{
		"code":    errno.OK.FullCode(),
		"userMsg": errno.OK.Message,
		"data":    data,
	}

	if err == nil {
		if config.IsDev() {
			resp["devMsg"] = ""
		}
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

	if config.IsDev() {
		resp["devMsg"] = e.Error()
	}

	c.JSON(http.StatusOK, resp)
}
