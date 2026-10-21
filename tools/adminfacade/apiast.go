package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
)

type apiPackage struct {
	fset       *token.FileSet
	types      map[string]*ast.TypeSpec
	methods    map[string]map[string]*ast.FuncDecl
	consts     map[string][]string
	operations []*apiOperation
}

type apiOperation struct {
	Name         string
	ParamsType   string
	RequestType  string
	ResponseType string
	Info         specOperation
	Service      serviceDef
	Method       string
}

type fieldInfo struct {
	Name     string
	Type     ast.Expr
	Tag      string
	JSONName string
	HasJSON  bool
}

type responseInfo struct {
	BodyType string
	ElemType string
	IsArray  bool
	IsStruct bool
	HasBody  bool
	HasNext  bool
}

// loadAPI parses the generated api package.
func loadAPI(dir string) (*apiPackage, error) {
	pkg := &apiPackage{
		fset:    token.NewFileSet(),
		types:   make(map[string]*ast.TypeSpec),
		methods: make(map[string]map[string]*ast.FuncDecl),
		consts:  make(map[string][]string),
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(pkg.fset, name, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				switch decl.Tok {
				case token.TYPE:
					for _, spec := range decl.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						pkg.types[typeSpec.Name.Name] = typeSpec
					}
				case token.CONST:
					lastType := ""
					for _, spec := range decl.Specs {
						valueSpec, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						if valueSpec.Type != nil {
							if ident, ok := valueSpec.Type.(*ast.Ident); ok {
								lastType = ident.Name
							}
						}
						if lastType == "" {
							continue
						}
						for _, name := range valueSpec.Names {
							pkg.consts[lastType] = append(pkg.consts[lastType], name.Name)
						}
					}
				}
			case *ast.FuncDecl:
				if decl.Recv == nil || len(decl.Recv.List) != 1 {
					continue
				}
				receiver := receiverName(decl.Recv.List[0].Type)
				if receiver == "" {
					continue
				}
				if pkg.methods[receiver] == nil {
					pkg.methods[receiver] = make(map[string]*ast.FuncDecl)
				}
				pkg.methods[receiver][decl.Name.Name] = decl
			}
		}
	}
	if err := pkg.collectOperations(); err != nil {
		return nil, err
	}
	return pkg, nil
}

func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.Ident:
		return expr.Name
	default:
		return ""
	}
}

func (p *apiPackage) collectOperations() error {
	for name, fn := range p.methods["Client"] {
		if !ast.IsExported(name) {
			continue
		}
		op, ok, err := p.parseOperation(name, fn)
		if err != nil {
			return err
		}
		if ok {
			p.operations = append(p.operations, op)
		}
	}
	if len(p.operations) == 0 {
		return fmt.Errorf("no operations found on api.Client")
	}
	return nil
}

func (p *apiPackage) parseOperation(name string, fn *ast.FuncDecl) (*apiOperation, bool, error) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return nil, false, nil
	}
	first := fn.Type.Params.List[0]
	if !isSelector(first.Type, "context", "Context") {
		return nil, false, nil
	}
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 2 {
		return nil, false, nil
	}
	responseIdent, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	if !ok || !strings.HasSuffix(responseIdent.Name, "Res") {
		return nil, false, nil
	}
	op := &apiOperation{Name: name, ResponseType: responseIdent.Name}
	for _, field := range fn.Type.Params.List[1:] {
		switch expr := field.Type.(type) {
		case *ast.StarExpr:
			if ident, ok := expr.X.(*ast.Ident); ok {
				op.RequestType = ident.Name
			}
		case *ast.Ident:
			if strings.HasSuffix(expr.Name, "Params") {
				op.ParamsType = expr.Name
			}
		}
	}
	if op.ParamsType == "" {
		return nil, false, fmt.Errorf("%s: no params type", name)
	}
	return op, true, nil
}

// fields returns the fields of a struct type, resolving defined-type aliases
// such as `type CreateDomainOK DomainHeaders`.
func (p *apiPackage) fields(typeName string) ([]fieldInfo, error) {
	typeSpec := p.types[typeName]
	if typeSpec == nil {
		return nil, fmt.Errorf("type %s not found", typeName)
	}
	switch underlying := typeSpec.Type.(type) {
	case *ast.StructType:
		var fields []fieldInfo
		for _, field := range underlying.Fields.List {
			names := field.Names
			if len(names) == 0 {
				continue
			}
			tag := ""
			if field.Tag != nil {
				tag = reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
			}
			jsonName, _, _ := strings.Cut(tag, ",")
			for _, name := range names {
				fields = append(fields, fieldInfo{
					Name:     name.Name,
					Type:     field.Type,
					Tag:      tag,
					JSONName: jsonName,
					HasJSON:  tag != "",
				})
			}
		}
		return fields, nil
	case *ast.Ident:
		return p.fields(underlying.Name)
	default:
		return nil, fmt.Errorf("type %s is not a struct", typeName)
	}
}

func (p *apiPackage) field(typeName, fieldName string) (fieldInfo, bool) {
	fields, err := p.fields(typeName)
	if err != nil {
		return fieldInfo{}, false
	}
	for _, field := range fields {
		if field.Name == fieldName {
			return field, true
		}
	}
	return fieldInfo{}, false
}

func (p *apiPackage) responseInfo(op *apiOperation) (responseInfo, error) {
	typeSpec := p.types[op.ResponseType]
	if typeSpec == nil {
		return responseInfo{}, fmt.Errorf("%s: response type %s not found", op.Name, op.ResponseType)
	}
	iface, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok || iface.Methods == nil || len(iface.Methods.List) == 0 {
		return responseInfo{}, fmt.Errorf("%s: response type %s is not an interface", op.Name, op.ResponseType)
	}
	marker := iface.Methods.List[0]
	if len(marker.Names) == 0 {
		return responseInfo{}, fmt.Errorf("%s: response marker missing", op.Name)
	}
	markerName := marker.Names[0].Name

	var info responseInfo
	var bodyType ast.Expr
	noContentSuccess := false
	for typeName, methods := range p.methods {
		if methods[markerName] == nil {
			continue
		}
		fields, err := p.fields(typeName)
		if err != nil {
			continue
		}
		var responseField *fieldInfo
		hasNext := false
		for i := range fields {
			switch fields[i].Name {
			case "Response":
				responseField = &fields[i]
			case "XNextPageToken":
				hasNext = true
			}
		}
		if responseField == nil {
			noContentSuccess = true
			continue
		}
		if ident, ok := responseField.Type.(*ast.Ident); ok && ident.Name == "Problem" {
			continue
		}
		if bodyType == nil {
			bodyType = responseField.Type
		} else if p.qualified(bodyType) != p.qualified(responseField.Type) {
			return responseInfo{}, fmt.Errorf("%s: inconsistent success bodies %s and %s", op.Name, p.qualified(bodyType), p.qualified(responseField.Type))
		}
		info.HasNext = info.HasNext || hasNext
	}
	if bodyType == nil {
		if noContentSuccess {
			return responseInfo{}, nil
		}
		return responseInfo{}, fmt.Errorf("%s: no success response", op.Name)
	}
	info.HasBody = true
	info.BodyType = p.qualified(bodyType)
	if array, ok := bodyType.(*ast.ArrayType); ok {
		info.IsArray = true
		info.ElemType = p.qualified(array.Elt)
	} else {
		info.IsStruct = p.isStructType(bodyType)
	}
	return info, nil
}

func (p *apiPackage) isStructType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	typeSpec := p.types[ident.Name]
	if typeSpec == nil {
		return false
	}
	switch underlying := typeSpec.Type.(type) {
	case *ast.StructType:
		return true
	case *ast.Ident:
		return p.isStructType(underlying)
	default:
		return false
	}
}

func (p *apiPackage) optValueType(optName string) (ast.Expr, bool) {
	field, ok := p.field(optName, "Value")
	if !ok {
		return nil, false
	}
	return field.Type, true
}

// underlying resolves defined-type aliases to their underlying expression.
func (p *apiPackage) underlying(expr ast.Expr) ast.Expr {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return expr
	}
	typeSpec := p.types[ident.Name]
	if typeSpec == nil {
		return expr
	}
	return p.underlying(typeSpec.Type)
}

func (p *apiPackage) isStringLike(expr ast.Expr) bool {
	ident, ok := p.underlying(expr).(*ast.Ident)
	return ok && ident.Name == "string"
}

func (p *apiPackage) isBoolLike(expr ast.Expr) bool {
	ident, ok := p.underlying(expr).(*ast.Ident)
	return ok && ident.Name == "bool"
}

func (p *apiPackage) isNumeric(expr ast.Expr) bool {
	ident, ok := p.underlying(expr).(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return true
	default:
		return false
	}
}

func (p *apiPackage) isSliceType(expr ast.Expr) bool {
	_, ok := p.underlying(expr).(*ast.ArrayType)
	return ok
}

// responseTypes returns the generated response wrapper and alias types.
func (p *apiPackage) responseTypes() map[string]bool {
	types := make(map[string]bool)
	for receiver, methods := range p.methods {
		for name, fn := range methods {
			if strings.HasSuffix(name, "Res") && (fn.Type.Params == nil || len(fn.Type.Params.List) == 0) &&
				(fn.Type.Results == nil || len(fn.Type.Results.List) == 0) {
				types[receiver] = true
			}
		}
	}
	return types
}

// isEnumType reports whether a type has a string, numeric, or boolean
// underlying type.
func (p *apiPackage) isEnumType(name string) bool {
	typeSpec := p.types[name]
	if typeSpec == nil {
		return false
	}
	return p.isStringLike(typeSpec.Type) || p.isNumeric(typeSpec.Type) || p.isBoolLike(typeSpec.Type)
}

// qualified renders a type expression for the admin package, prefixing api
// package types.
func (p *apiPackage) qualified(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		if isBuiltin(expr.Name) {
			return expr.Name
		}
		return "api." + expr.Name
	case *ast.SelectorExpr:
		return p.exprString(expr)
	case *ast.StarExpr:
		return "*" + p.qualified(expr.X)
	case *ast.ArrayType:
		if expr.Len == nil {
			return "[]" + p.qualified(expr.Elt)
		}
		return "[" + p.exprString(expr.Len) + "]" + p.qualified(expr.Elt)
	case *ast.MapType:
		return "map[" + p.qualified(expr.Key) + "]" + p.qualified(expr.Value)
	case *ast.InterfaceType:
		return "any"
	case *ast.ParenExpr:
		return "(" + p.qualified(expr.X) + ")"
	case *ast.Ellipsis:
		return "..." + p.qualified(expr.Elt)
	default:
		return p.exprString(expr)
	}
}

func (p *apiPackage) exprString(expr ast.Expr) string {
	var builder strings.Builder
	_ = printer.Fprint(&builder, p.fset, expr)
	return builder.String()
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == pkg && selector.Sel.Name == name
}

var builtins = map[string]bool{
	"any": true, "bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true, "int8": true,
	"int16": true, "int32": true, "int64": true, "rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true,
}

func isBuiltin(name string) bool {
	return builtins[name]
}
