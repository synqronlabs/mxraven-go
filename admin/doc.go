// Package admin is a Go client for the mxRaven control-plane v2 HTTP API.
//
// The transport in the admin/api subpackage is generated from a tenant-only
// subset of the authoritative OpenAPI contract. Platform-administrator,
// account self-service, billing, and signup operations are intentionally
// excluded. This package adds the cross-cutting concerns that are awkward to
// express in generated code: client construction, bearer authentication,
// rate-limit handling, RFC 9457 problem extraction, and pagination.
//
// Every generated operation is available through the embedded
// [github.com/synqronlabs/mxraven-go/admin/api.Client]. A minimal call looks
// like:
//
//	client, err := admin.New(
//		admin.WithBaseURL("https://control.example.com"),
//		admin.WithToken(token),
//	)
//	if err != nil {
//		return err
//	}
//
//	res, err := client.GetTenant(ctx, api.GetTenantParams{Slug: "acme"})
//	if err != nil {
//		return err
//	}
//	if problem, ok := admin.ProblemFrom(res); ok {
//		return fmt.Errorf("get tenant: %s", problem.Code)
//	}
//	tenant := res.(*api.TenantHeaders).Response
//
// Problem responses are decoded as typed generated values rather than Go
// errors. Use [ProblemFrom] to extract the RFC 9457 problem details from any
// response value.
//
// The proprietary contract under openapi/ is not committed. Copy it from the
// proprietary source for development, then regenerate the committed transport
// in admin/api with:
//
//	go generate ./admin
//
//go:generate go run github.com/synqronlabs/mxraven-go/tools/openapifilter -input ../openapi/v2 -output ../openapi/public/v2
//go:generate go run github.com/ogen-go/ogen/cmd/ogen@v1.24.0 -config ogen.yaml -target api -package api -clean ../openapi/public/v2/openapi.yaml
package admin
