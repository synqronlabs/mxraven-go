// Package admin is a Go client for the mxRaven control-plane v2 HTTP API.
//
// The transport in the admin/api subpackage is generated from a tenant-only
// subset of the authoritative OpenAPI contract. Platform-administrator,
// account self-service, billing, and signup operations are intentionally
// excluded. This package adds the concerns that are awkward to express in
// generated code: client construction, bearer authentication, typed API errors,
// tenant-bound services, lazy pagination, and terminal-action payload helpers.
//
// Most applications use a tenant-bound [Workspace]:
//
//	client, err := admin.New(admin.WithToken(token))
//	if err != nil {
//		return err
//	}
//
//	ws := client.Workspace("acme")
//	tenant, err := ws.Tenant.Get(ctx)
//	if err != nil {
//		return err
//	}
//	fmt.Println(tenant.Slug, tenant.Status)
//
//	for domain, err := range ws.Domains.List(ctx, admin.ListDomainsOptions{}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(domain.DomainName)
//	}
//
// List operations return lazy iter.Seq2 values. Pages are fetched only as the
// sequence is consumed, and stopping early stops further requests. Use [Take] or
// [Collect] when a slice is more convenient, or the generated Pages methods when
// explicit page control is needed.
//
// Facade-visible models and enums are aliased in this package, so most programs
// can use admin.Domain, admin.Listener, and admin.DomainStatusVerified without
// importing the generated api package directly.
//
// Non-success responses are converted to [APIError] by the tenant-bound
// services. Use errors.As or [AsAPIError] to inspect the stable problem code:
//
//	var apiErr *admin.APIError
//	if errors.As(err, &apiErr) && apiErr.Code == "not_found" {
//		// Handle a missing resource.
//	}
//
// Every generated operation remains available through the embedded
// [github.com/synqronlabs/mxraven-go/admin/api.Client]. Use [Do] and [Check] to
// unwrap generated results while still receiving typed API errors. The
// tenant-bound service layer is maintained with tools/adminfacade.
//
// The proprietary contract under openapi/ is not committed. Copy it from the
// proprietary source for development, then regenerate the committed transport
// and facade in admin with:
//
//	go generate ./admin
//
//go:generate go run github.com/synqronlabs/mxraven-go/tools/openapifilter -input ../openapi/v2 -output ../openapi/public/v2
//go:generate go run github.com/ogen-go/ogen/cmd/ogen@v1.24.0 -config ogen.yaml -target api -package api -clean ../openapi/public/v2/openapi.yaml
//go:generate go run github.com/synqronlabs/mxraven-go/tools/adminfacade -api ./api -openapi ../openapi/public/v2 -output .
package admin
