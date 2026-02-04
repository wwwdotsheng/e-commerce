package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Data:    data,
		Message: "ok",
	})
}

func Fail(c *gin.Context, httpCode int, data interface{}, msg string) {
	c.JSON(httpCode, Response{
		Code:    httpCode,
		Data:    data,
		Message: msg,
	})
}

func InternalError(c *gin.Context) {
	Fail(c, http.StatusInternalServerError, nil, "系统错误")
}

func BadRequest(c *gin.Context) {
	Fail(c, http.StatusBadRequest, nil, "参数错误")
}
