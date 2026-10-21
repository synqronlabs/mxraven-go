package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type generator struct {
	api      *apiPackage
	spec     *specInfo
	services map[string]serviceDef
	ops      []*facadeOperation
	manifest map[string]string
}

type facadeOperation struct {
	API             *apiOperation
	Service         serviceDef
	Method          string
	PathParams      []pathParam
	RequiredQuery   []queryField
	OptionalQuery   []queryField
	Response        responseInfo
	List            bool
	ListOptionsName string
	QueryName       string
}

type pathParam struct {
	Field string
	Arg   string
	Type  string
}

type queryField struct {
	Field    string
	Type     string
	Raw      string
	JSON     string
	Required bool
	Kind     string
	Ctor     string
}

func build(api *apiPackage, spec *specInfo) (*generator, error) {
	g := &generator{
		api:      api,
		spec:     spec,
		services: make(map[string]serviceDef),
		manifest: make(map[string]string),
	}
	for _, service := range tagServices {
		g.services[service.Key] = service
	}
	g.services[dedicatedIPPoolsService.Key] = dedicatedIPPoolsService

	knownOperations := make(map[string]bool, len(api.operations))
	for _, op := range api.operations {
		knownOperations[op.Name] = true
	}
	for name := range spec.Operations {
		if !knownOperations[name] {
			return nil, fmt.Errorf("OpenAPI operation %s has no generated client method", name)
		}
	}

	methods := make(map[string]map[string]string)
	for _, op := range api.operations {
		meta, ok := spec.Operations[op.Name]
		if !ok {
			return nil, fmt.Errorf("operation %s has no OpenAPI metadata", op.Name)
		}
		op.Info = meta
		service, err := g.serviceFor(op)
		if err != nil {
			return nil, err
		}
		if service.Tag == "" {
			service.Tag = meta.Tag
		}
		g.services[service.Key] = service
		op.Service = service
		op.Method = methodName(op.Name, service)
		if target, ok := handwrittenOperations[op.Name]; ok {
			g.manifest[op.Name] = target
			continue
		}
		facade, err := g.buildOperation(op)
		if err != nil {
			return nil, err
		}
		if methods[service.Key] == nil {
			methods[service.Key] = make(map[string]string)
		}
		if previous, ok := methods[service.Key][facade.Method]; ok {
			return nil, fmt.Errorf("operation %s and %s both map to %s.%s", previous, op.Name, service.Field, facade.Method)
		}
		methods[service.Key][facade.Method] = op.Name
		g.manifest[op.Name] = service.Field + "." + facade.Method
		g.ops = append(g.ops, facade)
	}
	return g, nil
}

func (g *generator) serviceFor(op *apiOperation) (serviceDef, error) {
	if field, ok := serviceOverrides[op.Name]; ok {
		for _, service := range g.services {
			if service.Field == field {
				return service, nil
			}
		}
	}
	if service, ok := tagServices[op.Info.Tag]; ok {
		return service, nil
	}
	field := pascal(splitWords(op.Info.Tag))
	if field == "" {
		return serviceDef{}, fmt.Errorf("operation %s has no service tag", op.Name)
	}
	service := serviceDef{
		Key:   camelFromSnake(snake(field)),
		Field: field,
		Type:  field + "Service",
	}
	g.services[service.Key] = service
	return service, nil
}

func (g *generator) buildOperation(op *apiOperation) (*facadeOperation, error) {
	response, err := g.api.responseInfo(op)
	if err != nil {
		return nil, err
	}
	facade := &facadeOperation{API: op, Service: op.Service, Method: op.Method, Response: response}
	facade.List = response.IsArray && response.HasNext
	if facade.List {
		facade.ListOptionsName = op.Name + "Options"
	}

	fields, err := g.api.fields(op.ParamsType)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op.Name, err)
	}
	pathNames := pathParameters(op.Info.Path)
	pathFields := make(map[string]bool)
	for _, name := range pathNames {
		if name == "slug" || name == "tenant_slug" {
			continue
		}
		field, ok := findPathField(fields, name)
		if !ok {
			return nil, fmt.Errorf("%s: no params field for path parameter %q", op.Name, name)
		}
		pathFields[field.Name] = true
		facade.PathParams = append(facade.PathParams, pathParam{
			Field: field.Name,
			Arg:   camelFromSnake(name),
			Type:  g.api.qualified(field.Type),
		})
	}

	for _, field := range fields {
		if field.Name == "Slug" || pathFields[field.Name] {
			continue
		}
		if facade.List && (field.Name == "PageSize" || field.Name == "PageToken" || field.JSONName == "page_size" || field.JSONName == "page_token") {
			continue
		}
		query, err := g.queryField(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op.Name, err)
		}
		if field.HasJSON {
			facade.OptionalQuery = append(facade.OptionalQuery, query)
		} else {
			facade.RequiredQuery = append(facade.RequiredQuery, query)
		}
	}
	if !facade.List && len(facade.RequiredQuery)+len(facade.OptionalQuery) > 0 {
		facade.QueryName = op.Name + "Query"
	}
	return facade, nil
}

func findPathField(fields []fieldInfo, name string) (fieldInfo, bool) {
	want := pascalFromSnake(name)
	for _, field := range fields {
		if field.HasJSON {
			continue
		}
		if strings.EqualFold(field.Name, want) {
			return field, true
		}
	}
	return fieldInfo{}, false
}

func (g *generator) queryField(field fieldInfo) (queryField, error) {
	jsonName := field.JSONName
	fieldName := field.Name
	if jsonName != "" {
		fieldName = pascalFromSnake(jsonName)
	} else {
		jsonName = field.Name
	}
	if jsonName == "q" || field.Name == "Q" {
		if field.HasJSON {
			return queryField{Field: "Search", Type: "string", Raw: field.Name, JSON: jsonName, Kind: "search"}, nil
		}
		return queryField{Field: "Search", Type: "string", Raw: field.Name, JSON: jsonName, Required: true, Kind: "searchRequired"}, nil
	}
	if !field.HasJSON {
		return queryField{
			Field:    field.Name,
			Type:     g.api.qualified(field.Type),
			Raw:      field.Name,
			JSON:     jsonName,
			Required: true,
			Kind:     "required",
		}, nil
	}
	query := queryField{
		Field: fieldName,
		Raw:   field.Name,
		JSON:  jsonName,
	}
	if ident, ok := field.Type.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "Opt") {
		query.Ctor = "api.New" + ident.Name
		valueType, ok := g.api.optValueType(ident.Name)
		if !ok || strings.HasPrefix(ident.Name, "OptNil") {
			query.Type = "api." + ident.Name
			query.Kind = "opt"
			return query, nil
		}
		switch ident.Name {
		case "OptString":
			query.Type = "string"
			query.Kind = "string"
		case "OptBool":
			query.Type = "*bool"
			query.Kind = "pointer"
		case "OptDateTime":
			query.Type = "*time.Time"
			query.Kind = "pointer"
		case "OptUUID":
			query.Type = "*uuid.UUID"
			query.Kind = "pointer"
		case "OptInt":
			query.Type = "*int"
			query.Kind = "pointer"
		case "OptInt32":
			query.Type = "*int32"
			query.Kind = "pointer"
		case "OptInt64":
			query.Type = "*int64"
			query.Kind = "pointer"
		case "OptFloat32":
			query.Type = "*float32"
			query.Kind = "pointer"
		case "OptFloat64":
			query.Type = "*float64"
			query.Kind = "pointer"
		default:
			switch {
			case g.api.isStringLike(valueType):
				query.Type = g.api.qualified(valueType)
				query.Kind = "enum"
			case g.api.isNumeric(valueType), g.api.isBoolLike(valueType):
				query.Type = "*" + g.api.qualified(valueType)
				query.Kind = "pointer"
			case g.api.isSliceType(valueType):
				query.Type = g.api.qualified(valueType)
				query.Kind = "slice"
			default:
				query.Type = "api." + ident.Name
				query.Kind = "opt"
			}
		}
		return query, nil
	}
	if g.api.isSliceType(field.Type) {
		query.Type = g.api.qualified(field.Type)
		query.Kind = "slice"
		return query, nil
	}
	query.Type = g.api.qualified(field.Type)
	query.Kind = "opt"
	return query, nil
}

// render generates all facade files.
func (g *generator) render() (map[string][]byte, error) {
	files := make(map[string][]byte)
	byService := make(map[string][]*facadeOperation)
	for _, op := range g.ops {
		byService[op.Service.Key] = append(byService[op.Service.Key], op)
	}
	keys := make([]string, 0, len(byService))
	for key := range byService {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		service := g.services[key]
		if service.Handwritten {
			continue
		}
		source, err := g.renderService(service, byService[key])
		if err != nil {
			return nil, err
		}
		files["facade_"+snake(service.Field)+"_gen.go"] = source
	}
	workspace, err := g.renderWorkspaceServices()
	if err != nil {
		return nil, err
	}
	files["workspace_services_gen.go"] = workspace
	manifest, err := g.renderManifest()
	if err != nil {
		return nil, err
	}
	files["facade_manifest_gen.go"] = manifest
	return files, nil
}

func (g *generator) renderService(service serviceDef, ops []*facadeOperation) ([]byte, error) {
	sort.Slice(ops, func(i, j int) bool { return ops[i].Method < ops[j].Method })
	var body bytes.Buffer
	hasList := false
	needsTime := false
	needsUUID := false
	for _, op := range ops {
		if op.List {
			hasList = true
		}
		text := g.renderOperation(op)
		if strings.Contains(text, "time.") {
			needsTime = true
		}
		if strings.Contains(text, "uuid.") {
			needsUUID = true
		}
		body.WriteString(text)
		body.WriteString("\n")
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by adminfacade; DO NOT EDIT.\n\npackage admin\n\n")
	out.WriteString("import (\n")
	out.WriteString("\t\"context\"\n")
	if hasList {
		out.WriteString("\t\"iter\"\n")
	}
	if needsTime {
		out.WriteString("\t\"time\"\n")
	}
	out.WriteString("\n")
	if needsUUID {
		out.WriteString("\t\"github.com/google/uuid\"\n\n")
	}
	out.WriteString("\t\"github.com/synqronlabs/mxraven-go/admin/api\"\n")
	out.WriteString(")\n\n")
	display := service.Tag
	if display == "" {
		display = service.Field
	}
	out.WriteString("// " + service.Type + " provides access to the " + display + " resource family.\n")
	out.WriteString("//\n")
	out.WriteString("// Obtain one from [Workspace]. It is safe for concurrent use.\n")
	out.WriteString("type " + service.Type + " struct {\n")
	out.WriteString("\tclient *Client\n")
	out.WriteString("\tslug   string\n")
	out.WriteString("}\n\n")
	out.Write(body.Bytes())

	source, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format %s: %w\n%s", service.Type, err, out.String())
	}
	return source, nil
}

func (g *generator) renderOperation(op *facadeOperation) string {
	var out bytes.Buffer
	out.WriteString("// " + op.Method + " calls the " + op.API.Name + " operation.")
	if summary := strings.TrimSpace(op.API.Info.Summary); summary != "" {
		out.WriteString("\n//\n// Summary: " + summary)
	}
	if op.List {
		out.WriteString("\n//\n// Pages are fetched lazily as the sequence is consumed.")
	}
	out.WriteString("\n")
	out.WriteString("func (s *" + op.Service.Type + ") " + op.Method + "(")
	args := []string{"ctx context.Context"}
	for _, param := range op.PathParams {
		args = append(args, param.Arg+" "+param.Type)
	}
	if op.API.RequestType != "" {
		args = append(args, "request api."+op.API.RequestType)
	}
	switch {
	case op.List:
		args = append(args, "opts "+op.ListOptionsName)
	case op.QueryName != "":
		args = append(args, "query "+op.QueryName)
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(") ")
	switch {
	case op.List:
		out.WriteString("iter.Seq2[" + op.Response.ElemType + ", error]")
	case !op.Response.HasBody:
		out.WriteString("error")
	case op.Response.IsArray:
		out.WriteString("([]" + op.Response.ElemType + ", error)")
	case op.Response.IsStruct:
		out.WriteString("(*" + op.Response.BodyType + ", error)")
	default:
		out.WriteString("(" + op.Response.BodyType + ", error)")
	}
	out.WriteString(" {\n")
	out.WriteString("\tparams := api." + op.API.ParamsType + "{Slug: api.TenantSlug(s.slug)}\n")
	for _, param := range op.PathParams {
		out.WriteString("\tparams." + param.Field + " = " + param.Arg + "\n")
	}
	if op.List {
		fetchName := "fetch" + op.API.Name + "Page"
		pagesName := pageMethodName(op.Method)
		out.WriteString("\tif err := opts.apply(&params); err != nil {\n")
		out.WriteString("\t\treturn errorSeq[" + op.Response.ElemType + "](err)\n")
		out.WriteString("\t}\n")
		out.WriteString("\treturn paginate(ctx, opts.PageToken, func(ctx context.Context, token string) ([]" + op.Response.ElemType + ", string, error) {\n")
		out.WriteString("\t\tpage, err := s." + fetchName + "(ctx, params, token)\n")
		out.WriteString("\t\tif err != nil {\n")
		out.WriteString("\t\t\treturn nil, \"\", err\n")
		out.WriteString("\t\t}\n")
		out.WriteString("\t\treturn page.Items, page.NextToken, nil\n")
		out.WriteString("\t})\n")
		out.WriteString("}\n\n")

		out.WriteString("// " + pagesName + " returns a page-level iterator over the " + op.API.Name + " operation.\n")
		out.WriteString("//\n")
		out.WriteString("// The first page is fetched when Next is called. Use [Pager.Page] for\n")
		out.WriteString("// the cursor metadata of the most recently loaded page.\n")
		pageArgs := strings.Join(args[1:], ", ")
		out.WriteString("func (s *" + op.Service.Type + ") " + pagesName + "(" + pageArgs + ") *Pager[" + op.Response.ElemType + "] {\n")
		out.WriteString("\tparams := api." + op.API.ParamsType + "{Slug: api.TenantSlug(s.slug)}\n")
		for _, param := range op.PathParams {
			out.WriteString("\tparams." + param.Field + " = " + param.Arg + "\n")
		}
		out.WriteString("\tif err := opts.apply(&params); err != nil {\n")
		out.WriteString("\t\treturn errorPager[" + op.Response.ElemType + "](err)\n")
		out.WriteString("\t}\n")
		out.WriteString("\treturn newPager(func(ctx context.Context, token string) (Page[" + op.Response.ElemType + "], error) {\n")
		out.WriteString("\t\treturn s." + fetchName + "(ctx, params, token)\n")
		out.WriteString("\t})\n")
		out.WriteString("}\n\n")

		out.WriteString("func (s *" + op.Service.Type + ") " + fetchName + "(ctx context.Context, params api." + op.API.ParamsType + ", token string) (Page[" + op.Response.ElemType + "], error) {\n")
		out.WriteString("\tpageParams := params\n")
		out.WriteString("\tif token != \"\" {\n")
		out.WriteString("\t\tpageParams.PageToken = api.NewOptString(token)\n")
		out.WriteString("\t}\n")
		out.WriteString("\tres, err := s.client." + op.API.Name + "(ctx, pageParams)\n")
		out.WriteString("\tif err != nil {\n")
		out.WriteString("\t\treturn Page[" + op.Response.ElemType + "]{}, err\n")
		out.WriteString("\t}\n")
		out.WriteString("\tpage, ok := res.(pageResponse[" + op.Response.ElemType + "])\n")
		out.WriteString("\tif !ok {\n")
		out.WriteString("\t\treturn Page[" + op.Response.ElemType + "]{}, problemError(res)\n")
		out.WriteString("\t}\n")
		out.WriteString("\tnext, _ := NextPageToken(res)\n")
		out.WriteString("\tprevious, _ := PreviousPageToken(res)\n")
		out.WriteString("\tlast, _ := LastPageToken(res)\n")
		out.WriteString("\treturn Page[" + op.Response.ElemType + "]{Items: page.GetResponse(), NextToken: next, PreviousToken: previous, LastToken: last}, nil\n")
		out.WriteString("}\n\n")
		g.renderOptions(&out, op)
		return out.String()
	}
	if op.QueryName != "" {
		out.WriteString("\tif err := query.apply(&params); err != nil {\n")
		if op.Response.HasBody {
			out.WriteString("\t\tvar zero " + responseValueType(op) + "\n")
			out.WriteString("\t\treturn zero, err\n")
		} else {
			out.WriteString("\t\treturn err\n")
		}
		out.WriteString("\t}\n")
	}
	if op.API.RequestType != "" {
		out.WriteString("\tres, err := s.client." + op.API.Name + "(ctx, &request, params)\n")
	} else {
		out.WriteString("\tres, err := s.client." + op.API.Name + "(ctx, params)\n")
	}
	switch {
	case !op.Response.HasBody:
		out.WriteString("\treturn Check(res, err)\n")
	case op.Response.IsArray:
		out.WriteString("\treturn Do[[]" + op.Response.ElemType + "](res, err)\n")
	case op.Response.IsStruct:
		out.WriteString("\treturn resultPtr[" + op.Response.BodyType + "](res, err)\n")
	default:
		out.WriteString("\treturn Do[" + op.Response.BodyType + "](res, err)\n")
	}
	out.WriteString("}\n\n")
	if op.QueryName != "" {
		g.renderQuery(&out, op)
	}
	return out.String()
}

func (g *generator) renderOptions(out *bytes.Buffer, op *facadeOperation) {
	out.WriteString("// " + op.ListOptionsName + " filters and paginates [")
	out.WriteString(op.Service.Type + "." + op.Method + "].\n")
	out.WriteString("type " + op.ListOptionsName + " struct {\n")
	out.WriteString("\tListOptions\n")
	g.renderQueryFields(out, op)
	out.WriteString("}\n\n")
	out.WriteString("func (o " + op.ListOptionsName + ") apply(params *api." + op.API.ParamsType + ") error {\n")
	out.WriteString("\tif err := o.ListOptions.apply(&params.PageSize); err != nil {\n")
	out.WriteString("\t\treturn err\n")
	out.WriteString("\t}\n")
	g.renderApply(out, op, "o")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *generator) renderQuery(out *bytes.Buffer, op *facadeOperation) {
	out.WriteString("// " + op.QueryName + " contains query parameters for [")
	out.WriteString(op.Service.Type + "." + op.Method + "].\n")
	out.WriteString("type " + op.QueryName + " struct {\n")
	g.renderQueryFields(out, op)
	out.WriteString("}\n\n")
	out.WriteString("func (q " + op.QueryName + ") apply(params *api." + op.API.ParamsType + ") error {\n")
	g.renderApply(out, op, "q")
	out.WriteString("\treturn nil\n")
	out.WriteString("}\n\n")
}

func (g *generator) renderQueryFields(out *bytes.Buffer, op *facadeOperation) {
	for _, field := range append(append([]queryField{}, op.RequiredQuery...), op.OptionalQuery...) {
		if field.Field == "Search" {
			out.WriteString("\t// Search is a case-insensitive search term.\n")
		} else if field.Required {
			out.WriteString("\t// " + field.Field + " is required.\n")
		} else {
			out.WriteString("\t// " + field.Field + " filters by " + field.JSON + ".\n")
		}
		out.WriteString("\t" + field.Field + " " + field.Type + "\n")
	}
}

func (g *generator) renderApply(out *bytes.Buffer, op *facadeOperation, receiver string) {
	for _, field := range append(append([]queryField{}, op.RequiredQuery...), op.OptionalQuery...) {
		switch field.Kind {
		case "searchRequired":
			out.WriteString("\tsearch, err := normalizeRequiredSearch(" + receiver + "." + field.Field + ")\n")
			out.WriteString("\tif err != nil {\n")
			out.WriteString("\t\treturn err\n")
			out.WriteString("\t}\n")
			out.WriteString("\tparams." + field.Raw + " = search\n")
		case "search":
			out.WriteString("\tsearch, err := normalizeSearch(" + receiver + "." + field.Field + ")\n")
			out.WriteString("\tif err != nil {\n")
			out.WriteString("\t\treturn err\n")
			out.WriteString("\t}\n")
			out.WriteString("\tif search != \"\" {\n")
			out.WriteString("\t\tparams." + field.Raw + " = api.NewOptString(search)\n")
			out.WriteString("\t}\n")
		case "required":
			out.WriteString("\tparams." + field.Raw + " = " + receiver + "." + field.Field + "\n")
		case "string":
			out.WriteString("\tif " + receiver + "." + field.Field + " != \"\" {\n")
			out.WriteString("\t\tparams." + field.Raw + " = " + field.Ctor + "(" + receiver + "." + field.Field + ")\n")
			out.WriteString("\t}\n")
		case "enum":
			out.WriteString("\tif " + receiver + "." + field.Field + " != \"\" {\n")
			out.WriteString("\t\tparams." + field.Raw + " = " + field.Ctor + "(" + receiver + "." + field.Field + ")\n")
			out.WriteString("\t}\n")
		case "pointer":
			out.WriteString("\tif " + receiver + "." + field.Field + " != nil {\n")
			out.WriteString("\t\tparams." + field.Raw + " = " + field.Ctor + "(*" + receiver + "." + field.Field + ")\n")
			out.WriteString("\t}\n")
		case "slice":
			out.WriteString("\tif len(" + receiver + "." + field.Field + ") > 0 {\n")
			out.WriteString("\t\tparams." + field.Raw + " = " + receiver + "." + field.Field + "\n")
			out.WriteString("\t}\n")
		case "opt":
			out.WriteString("\tif " + receiver + "." + field.Field + ".Set {\n")
			out.WriteString("\t\tparams." + field.Raw + " = " + receiver + "." + field.Field + "\n")
			out.WriteString("\t}\n")
		}
	}
}

func responseValueType(op *facadeOperation) string {
	switch {
	case op.Response.IsArray:
		return "[]" + op.Response.ElemType
	case op.Response.IsStruct:
		return "*" + op.Response.BodyType
	default:
		return op.Response.BodyType
	}
}

func (g *generator) renderWorkspaceServices() ([]byte, error) {
	services := make([]serviceDef, 0, len(g.services))
	seen := make(map[string]bool)
	for _, service := range g.services {
		if seen[service.Key] {
			continue
		}
		seen[service.Key] = true
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Field < services[j].Field })

	var out bytes.Buffer
	out.WriteString("// Code generated by adminfacade; DO NOT EDIT.\n\npackage admin\n\n")
	out.WriteString("// WorkspaceServices holds the tenant-bound service fields of a [Workspace].\n")
	out.WriteString("//\n")
	out.WriteString("// Obtain it from [Client.Workspace]; do not construct it directly.\n")
	out.WriteString("type WorkspaceServices struct {\n")
	for _, service := range services {
		display := service.Tag
		if display == "" {
			display = service.Field
		}
		out.WriteString("\t// " + service.Field + " provides access to the " + display + " resource family.\n")
		out.WriteString("\t" + service.Field + " *" + service.Type + "\n")
	}
	out.WriteString("}\n\n")
	out.WriteString("// newWorkspaceServices constructs the service set for a workspace.\n")
	out.WriteString("func newWorkspaceServices(client *Client, slug string) *WorkspaceServices {\n")
	out.WriteString("\treturn &WorkspaceServices{\n")
	for _, service := range services {
		out.WriteString("\t\t" + service.Field + ": &" + service.Type + "{client: client, slug: slug},\n")
	}
	out.WriteString("\t}\n")
	out.WriteString("}\n")
	source, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format workspace services: %w", err)
	}
	return source, nil
}

func (g *generator) renderManifest() ([]byte, error) {
	names := make([]string, 0, len(g.manifest))
	for name := range g.manifest {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	out.WriteString("// Code generated by adminfacade; DO NOT EDIT.\n\npackage admin\n\n")
	out.WriteString("// facadeOperations maps generated operation names to their facade method.\n")
	out.WriteString("var facadeOperations = map[string]string{\n")
	for _, name := range names {
		out.WriteString("\t" + fmt.Sprintf("%q", name) + ": " + fmt.Sprintf("%q", g.manifest[name]) + ",\n")
	}
	out.WriteString("}\n")
	source, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format manifest: %w", err)
	}
	return source, nil
}

// generatedFile reports whether name is managed by the facade generator.
func generatedFile(name string) bool {
	return (strings.HasPrefix(name, "facade_") && strings.HasSuffix(name, "_gen.go")) ||
		name == "workspace_services_gen.go" || name == "aliases_gen.go"
}

// renderAliases emits aliases for the API models and enums used by the facade.
func (g *generator) renderAliases(outputDir string, generated map[string][]byte) ([]byte, error) {
	refs := make(map[string]bool)
	adminNames := make(map[string]bool)

	files, err := filepath.Glob(filepath.Join(outputDir, "*.go"))
	if err != nil {
		return nil, err
	}
	for _, name := range files {
		base := filepath.Base(name)
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, "_gen.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		if err := collectAliasInfo(base, source, refs, adminNames); err != nil {
			return nil, err
		}
	}
	for name, source := range generated {
		if err := collectAliasInfo(name, source, refs, adminNames); err != nil {
			return nil, err
		}
	}

	responseTypes := g.api.responseTypes()
	aliases := make(map[string]bool)
	for name := range refs {
		if !ast.IsExported(name) || g.api.types[name] == nil || responseTypes[name] ||
			strings.HasSuffix(name, "Params") || strings.HasPrefix(name, "Opt") ||
			strings.HasPrefix(name, "Nil") || name == "OperationName" || name == "BearerAuth" ||
			adminNames[name] {
			continue
		}
		aliases[name] = true
	}
	for name := range g.api.types {
		if !ast.IsExported(name) || !g.api.isEnumType(name) || name == "OperationName" ||
			responseTypes[name] || adminNames[name] {
			continue
		}
		aliases[name] = true
	}
	typeNames := make([]string, 0, len(aliases))
	for name := range aliases {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	var constNames []string
	seen := make(map[string]bool)
	for _, typeName := range typeNames {
		for _, constName := range g.api.consts[typeName] {
			if !ast.IsExported(constName) || adminNames[constName] || seen[constName] {
				continue
			}
			seen[constName] = true
			constNames = append(constNames, constName)
		}
	}
	sort.Strings(constNames)
	if len(typeNames) == 0 && len(constNames) == 0 {
		return nil, nil
	}

	var out bytes.Buffer
	out.WriteString("// Code generated by adminfacade; DO NOT EDIT.\n\npackage admin\n\n")
	out.WriteString("import \"github.com/synqronlabs/mxraven-go/admin/api\"\n\n")
	for _, name := range typeNames {
		out.WriteString("// " + name + " is an alias for [api." + name + "].\n")
		out.WriteString("type " + name + " = api." + name + "\n\n")
	}
	if len(constNames) > 0 {
		out.WriteString("const (\n")
		for _, name := range constNames {
			out.WriteString("\t// " + name + " is an alias for [api." + name + "].\n")
			out.WriteString("\t" + name + " = api." + name + "\n")
		}
		out.WriteString(")\n")
	}
	source, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format aliases: %w", err)
	}
	return source, nil
}

func collectAliasInfo(name string, source []byte, refs, adminNames map[string]bool) error {
	file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok && ident.Name == "api" {
				refs[node.Sel.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					adminNames[spec.Name.Name] = true
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						adminNames[name.Name] = true
					}
				}
			}
		case *ast.FuncDecl:
			if node.Recv == nil {
				adminNames[node.Name.Name] = true
			}
		}
		return true
	})
	return nil
}

// writeFiles writes generated files and removes stale facade files.
func writeFiles(dir string, files map[string][]byte) error {
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), source, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if _, ok := files[name]; ok {
			continue
		}
		if generatedFile(name) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return fmt.Errorf("remove stale %s: %w", name, err)
			}
		}
	}
	return nil
}

func checkFiles(dir string, files map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !generatedFile(name) {
			continue
		}
		if _, ok := files[name]; !ok {
			return fmt.Errorf("%s is stale; run go generate ./admin", name)
		}
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("%s is stale; run go generate ./admin", name)
		}
	}
	return nil
}
