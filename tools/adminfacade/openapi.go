package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type specOperation struct {
	Path        string
	Tag         string
	Summary     string
	Description string
}

type specInfo struct {
	Operations      map[string]specOperation
	TagDescriptions map[string]string
}

type rootDocument struct {
	Paths map[string]pathItem `yaml:"paths"`
	Tags  []tagDocument       `yaml:"tags"`
}

type tagDocument struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type pathItem struct {
	Ref        string        `yaml:"$ref"`
	Parameters []any         `yaml:"parameters"`
	Get        *operationDoc `yaml:"get"`
	Post       *operationDoc `yaml:"post"`
	Put        *operationDoc `yaml:"put"`
	Patch      *operationDoc `yaml:"patch"`
	Delete     *operationDoc `yaml:"delete"`
	Head       *operationDoc `yaml:"head"`
	Options    *operationDoc `yaml:"options"`
	Trace      *operationDoc `yaml:"trace"`
}

type operationDoc struct {
	OperationID string   `yaml:"operationId"`
	Tags        []string `yaml:"tags"`
	Summary     string   `yaml:"summary"`
	Description string   `yaml:"description"`
}

// loadSpec reads the filtered OpenAPI contract and indexes operations by
// operation ID.
func loadSpec(dir string) (*specInfo, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "openapi.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read openapi.yaml: %w", err)
	}
	var root rootDocument
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse openapi.yaml: %w", err)
	}

	info := &specInfo{
		Operations:      make(map[string]specOperation),
		TagDescriptions: make(map[string]string),
	}
	for _, tag := range root.Tags {
		info.TagDescriptions[tag.Name] = strings.TrimSpace(tag.Description)
	}

	pathFiles := make(map[string]map[string]pathItem)
	for path, item := range root.Paths {
		if item.Ref != "" {
			fileName, key, ok := splitRef(item.Ref)
			if !ok {
				return nil, fmt.Errorf("path %q: invalid ref %q", path, item.Ref)
			}
			file, ok := pathFiles[fileName]
			if !ok {
				file, err = loadPathFile(dir, fileName)
				if err != nil {
					return nil, err
				}
				pathFiles[fileName] = file
			}
			resolved, ok := file[key]
			if !ok {
				return nil, fmt.Errorf("path %q: missing key %q in %s", path, key, fileName)
			}
			item = resolved
		}
		for _, op := range operationsOf(item) {
			if op.OperationID == "" {
				continue
			}
			tag := ""
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			name := pascal(splitWords(op.OperationID))
			if _, exists := info.Operations[name]; exists {
				return nil, fmt.Errorf("duplicate operation %s", op.OperationID)
			}
			info.Operations[name] = specOperation{
				Path:        path,
				Tag:         tag,
				Summary:     strings.TrimSpace(op.Summary),
				Description: strings.TrimSpace(op.Description),
			}
		}
	}
	return info, nil
}

func loadPathFile(dir, name string) (map[string]pathItem, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "paths", name))
	if err != nil {
		return nil, fmt.Errorf("read path file %s: %w", name, err)
	}
	var file map[string]pathItem
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse path file %s: %w", name, err)
	}
	return file, nil
}

func splitRef(ref string) (file, key string, ok bool) {
	filePart, keyPart, found := strings.Cut(ref, "#/")
	if !found {
		return "", "", false
	}
	return filepath.Base(filePart), strings.TrimPrefix(keyPart, "/"), true
}

func operationsOf(item pathItem) []*operationDoc {
	var ops []*operationDoc
	for _, op := range []*operationDoc{
		item.Get, item.Post, item.Put, item.Patch, item.Delete, item.Head, item.Options, item.Trace,
	} {
		if op != nil {
			ops = append(ops, op)
		}
	}
	return ops
}

// pathParameters returns the parameter names in a path template.
func pathParameters(path string) []string {
	var names []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return names
		}
		end := strings.IndexByte(path[start+1:], '}')
		if end < 0 {
			return names
		}
		names = append(names, path[start+1:start+1+end])
		path = path[start+end+2:]
	}
}
