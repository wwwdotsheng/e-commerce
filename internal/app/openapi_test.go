package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIIsValidYAML(t *testing.T) {
	router := gin.New()
	router.GET("/swagger/doc.yaml", swaggerDoc)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger/doc.yaml", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("Swagger 文档返回 %d: %s", response.Code, response.Body)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(response.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi.yaml 不是合法 YAML: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatal("Swagger 文档应包含 OpenAPI 映射")
	}
	root := doc.Content[0]
	if openAPIField(openAPIField(openAPIField(root, "paths"), "/order/{id}/refund"), "post") == nil {
		t.Fatal("合并后的 OpenAPI 文档缺少退款操作")
	}

	var checkReferences func(*yaml.Node)
	checkReferences = func(node *yaml.Node) {
		if ref := openAPIField(node, "$ref"); ref != nil {
			pointer, local := strings.CutPrefix(ref.Value, "#")
			if !local || openAPIPointer(root, pointer) == nil {
				t.Errorf("Swagger 文档中的引用无法在单文件内解析: %q", ref.Value)
			}
		}
		for _, child := range node.Content {
			checkReferences(child)
		}
	}
	checkReferences(root)
}

func TestOpenAPIUsesModuleReferences(t *testing.T) {
	source, err := openapiSource.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Paths map[string]struct {
			Ref string `yaml:"$ref"`
		}
		Components struct {
			Schemas map[string]struct {
				Ref string `yaml:"$ref"`
			}
		}
	}
	if err := yaml.Unmarshal(source, &index); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name string
		ref  string
	}{
		{"退款接口", index.Paths["/order/{id}/refund"].Ref},
		{"退款响应模型", index.Components.Schemas["RefundResponse"].Ref},
	} {
		if !strings.HasPrefix(entry.ref, "./openapi/order.yaml#") {
			t.Errorf("%s 应引用订单模块，实际引用 %q", entry.name, entry.ref)
		}
	}
}
