package app

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIIsValidYAML(t *testing.T) {
	var doc any
	if err := yaml.Unmarshal(openapiSpec, &doc); err != nil {
		t.Fatalf("openapi.yaml 不是合法 YAML: %v", err)
	}
}
