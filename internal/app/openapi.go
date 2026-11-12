package app

import (
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml openapi/*.yaml
var openapiSource embed.FS

var openapiSpec = sync.OnceValues(func() ([]byte, error) {
	return bundleOpenAPI(openapiSource)
})

func bundleOpenAPI(source fs.FS) ([]byte, error) {
	root, err := readOpenAPI(source, "openapi.yaml")
	if err != nil {
		return nil, err
	}

	// The index references module entries for paths and named schemas.
	sections := []*yaml.Node{
		openAPIField(root, "paths"),
		openAPIField(openAPIField(root, "components"), "schemas"),
	}
	modules := make(map[string]*yaml.Node)
	for _, section := range sections {
		if section == nil || section.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("OpenAPI index must contain paths and components.schemas mappings")
		}
		for i := 1; i < len(section.Content); i += 2 {
			entry := section.Content[i]
			ref := openAPIField(entry, "$ref")
			if ref == nil || !strings.HasPrefix(ref.Value, "./openapi/") {
				continue
			}
			filename, pointer, _ := strings.Cut(strings.TrimPrefix(ref.Value, "./"), "#")
			module := modules[filename]
			if module == nil {
				module, err = readOpenAPI(source, filename)
				if err != nil {
					return nil, err
				}
				rebaseOpenAPIModuleRefs(module)
				modules[filename] = module
			}
			target := openAPIPointer(module, pointer)
			if target == nil || target.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("OpenAPI module reference %q does not identify a mapping", ref.Value)
			}
			section.Content[i] = target
		}
	}
	return yaml.Marshal(root)
}

func readOpenAPI(source fs.FS, filename string) (*yaml.Node, error) {
	data, err := fs.ReadFile(source, filename)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must contain a YAML mapping", filename)
	}
	return doc.Content[0], nil
}

func openAPIField(node *yaml.Node, name string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == name {
			return node.Content[i+1]
		}
	}
	return nil
}

func openAPIPointer(node *yaml.Node, pointer string) *yaml.Node {
	pointer, err := url.PathUnescape(pointer)
	if err != nil || !strings.HasPrefix(pointer, "/") {
		return nil
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		name := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		node = openAPIField(node, name)
	}
	return node
}

func rebaseOpenAPIModuleRefs(node *yaml.Node) {
	if ref := openAPIField(node, "$ref"); ref != nil && strings.HasPrefix(ref.Value, "../openapi.yaml#/") {
		ref.Value = strings.TrimPrefix(ref.Value, "../openapi.yaml")
	}
	for _, child := range node.Content {
		rebaseOpenAPIModuleRefs(child)
	}
}
