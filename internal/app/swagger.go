package app

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var openapiSpec []byte

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>E-Commerce API</title>
  <style>
    body { margin: 0; }
  </style>
</head>
<body>
  <script
    id="api-reference"
    src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"
    data-url="/swagger/doc.yaml"
    data-hide-models="true"
    data-dark-mode="true"
    data-show-sidebar="true"
  ></script>
</body>
</html>`

func swaggerUI(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
}

func swaggerDoc(c *gin.Context) {
	c.Data(http.StatusOK, "text/yaml; charset=utf-8", openapiSpec)
}