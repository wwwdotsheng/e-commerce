package contextx

import (
	"e-commerce/internal/config"

	"github.com/gin-gonic/gin"
)

type configKeyStruct struct{}

var configKey = configKeyStruct{}

func GetConfig(c *gin.Context) *config.AppConfig {
	return c.MustGet(configKey).(*config.AppConfig)
}

func SetConfig(cfg *config.AppConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(configKey, cfg)
		c.Next()
	}
}
