// Command adminfacade generates the ergonomic tenant-bound facade for the
// generated admin/api client.
//
// The generator reads the generated api package for exact Go types and response
// shapes, and the filtered OpenAPI contract for operation paths and tags. It
// writes one facade file per generated resource family, a workspace service
// set, an operation coverage manifest, and aliases for the API models and enums
// used by the facade.
//
// Usage:
//
//	go run ./tools/adminfacade -api ./admin/api -openapi ./openapi/public/v2 -output ./admin
//
// Run with -check to verify that committed files are up to date.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	apiDir := flag.String("api", "./admin/api", "generated admin/api package directory")
	openapiDir := flag.String("openapi", "./openapi/public/v2", "filtered OpenAPI contract directory")
	outputDir := flag.String("output", "./admin", "facade output directory")
	check := flag.Bool("check", false, "verify generated files instead of writing them")
	flag.Parse()

	if err := run(*apiDir, *openapiDir, *outputDir, *check); err != nil {
		fmt.Fprintln(os.Stderr, "adminfacade:", err)
		os.Exit(1)
	}
}

func run(apiDir, openapiDir, outputDir string, check bool) error {
	api, err := loadAPI(apiDir)
	if err != nil {
		return err
	}
	spec, err := loadSpec(openapiDir)
	if err != nil {
		return err
	}
	gen, err := build(api, spec)
	if err != nil {
		return err
	}
	files, err := gen.render()
	if err != nil {
		return err
	}
	aliases, err := gen.renderAliases(outputDir, files)
	if err != nil {
		return err
	}
	if aliases != nil {
		files["aliases_gen.go"] = aliases
	}
	if check {
		return checkFiles(outputDir, files)
	}
	return writeFiles(outputDir, files)
}
