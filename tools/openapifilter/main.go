// Command openapifilter writes a tenant-only copy of the mxRaven control-plane
// v2 contract for SDK generation.
//
// The authoritative contract is proprietary and is copied into the local
// working tree for development only. The public admin SDK must not expose
// platform-administrator, account self-service, billing, or signup operations,
// so this tool derives a reduced contract that contains only tenant-bound
// operations.
//
// A path item is kept when at least one of its operations declares
// x-tenant-bound: true. The authoritative contract does not mix tenant-bound
// and non-tenant-bound operations within a single path item, so filtering at
// the path-item level is exact.
//
// Usage:
//
//	go run ./tools/openapifilter -input openapi/v2 -output openapi/public/v2
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var operationMethods = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "delete": {},
	"patch": {}, "options": {}, "head": {}, "trace": {},
}

func main() {
	input := flag.String("input", "openapi/v2", "full contract directory to read")
	output := flag.String("output", "openapi/public/v2", "directory to write the filtered contract")
	flag.Parse()

	if err := run(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, "openapifilter:", err)
		os.Exit(1)
	}
}

func run(input, output string) error {
	if input == "" {
		return fmt.Errorf("input directory must not be empty")
	}
	if output == "" {
		return fmt.Errorf("output directory must not be empty")
	}
	if err := copyTree(input, output); err != nil {
		return fmt.Errorf("copy contract: %w", err)
	}

	kept, err := filterPathFiles(filepath.Join(output, "paths"))
	if err != nil {
		return err
	}
	if err := filterRootDocument(filepath.Join(output, "openapi.yaml"), kept); err != nil {
		return err
	}

	total := 0
	for _, keys := range kept {
		total += len(keys)
	}
	fmt.Printf("openapifilter: kept %d tenant-bound path items in %s\n", total, output)
	return nil
}

// copyTree replaces dst with a copy of src.
func copyTree(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// filterPathFiles rewrites each path document in place, dropping path items
// that contain no tenant-bound operation. It returns the set of path item keys
// retained per file name.
func filterPathFiles(dir string) (map[string]map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read paths directory: %w", err)
	}

	kept := make(map[string]map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		document, root, err := loadMapping(path)
		if err != nil {
			return nil, err
		}

		filtered := make([]*yaml.Node, 0, len(root.Content))
		keptKeys := make(map[string]struct{})
		for i := 0; i+1 < len(root.Content); i += 2 {
			key, value := root.Content[i], root.Content[i+1]
			if !hasTenantOperation(value) {
				continue
			}
			filtered = append(filtered, key, value)
			keptKeys[key.Value] = struct{}{}
		}
		root.Content = filtered
		kept[entry.Name()] = keptKeys

		if err := writeDocument(path, document); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// filterRootDocument removes path references whose target key was filtered out
// of its path document.
func filterRootDocument(path string, kept map[string]map[string]struct{}) error {
	document, root, err := loadMapping(path)
	if err != nil {
		return err
	}
	paths := mappingValue(root, "paths")
	if paths == nil {
		return fmt.Errorf("%s: missing paths mapping", path)
	}

	filtered := make([]*yaml.Node, 0, len(paths.Content))
	for i := 0; i+1 < len(paths.Content); i += 2 {
		key, value := paths.Content[i], paths.Content[i+1]
		fileName, refKey, ok := parsePathRef(mappingValue(value, "$ref"))
		if !ok {
			filtered = append(filtered, key, value)
			continue
		}
		if _, ok := kept[fileName][refKey]; ok {
			filtered = append(filtered, key, value)
		}
	}
	paths.Content = filtered
	return writeDocument(path, document)
}

// parsePathRef splits a reference such as "./paths/domains.yaml#/domains".
func parsePathRef(ref *yaml.Node) (fileName, key string, ok bool) {
	if ref == nil || ref.Kind != yaml.ScalarNode {
		return "", "", false
	}
	filePart, keyPart, found := strings.Cut(ref.Value, "#")
	if !found {
		return "", "", false
	}
	key = strings.TrimPrefix(keyPart, "/")
	if key == "" || strings.Contains(key, "/") {
		return "", "", false
	}
	return filepath.Base(filePart), key, true
}

// hasTenantOperation reports whether a path item declares a tenant-bound
// operation.
func hasTenantOperation(item *yaml.Node) bool {
	if item == nil || item.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(item.Content); i += 2 {
		if _, ok := operationMethods[item.Content[i].Value]; !ok {
			continue
		}
		if scalarIsTrue(mappingValue(item.Content[i+1], "x-tenant-bound")) {
			return true
		}
	}
	return false
}

func scalarIsTrue(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Value == "true"
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// loadMapping reads a YAML document and returns it together with its root
// mapping node.
func loadMapping(path string) (document, root *yaml.Node, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	document = &yaml.Node{}
	if err := yaml.Unmarshal(data, document); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	root = document
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s: root is not a mapping", path)
	}
	return document, root, nil
}

func writeDocument(path string, document *yaml.Node) error {
	data, err := yaml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
