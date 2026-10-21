# mxRaven Admin SDK

A Go client for the mxRaven control-plane v2 HTTP API.

The SDK exposes tenant-scoped administration operations generated from the
authoritative OpenAPI contract, plus a tenant-bound service layer that provides
typed errors, lazy pagination, ergonomic filters, and resource-specific
helpers. The transport and most services are generated; the shared error,
pagination, and client core is hand-written.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/admin
```

Requires Go 1.26.4 or later (see `go.mod`).

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/synqronlabs/mxraven-go/admin"
)

func main() {
	client, err := admin.New(admin.WithToken("your-access-token"))
	if err != nil {
		log.Fatal(err)
	}

	ws := client.Workspace("acme")

	tenant, err := ws.Tenant.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(tenant.Slug, tenant.Status)

	domain, err := ws.Domains.Create(context.Background(), admin.CreateDomainInput{
		DomainName: "example.com",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(domain.ID, domain.Status)
}
```

The base URL defaults to `admin.DefaultBaseURL`; use `WithBaseURL` for staging
or self-hosted environments.

`Client` embeds the generated `*api.Client`, so every generated operation is
also available directly. The tenant-bound services add typed errors, lazy
pagination, and options without `Opt*` wrappers. The generated types and
operations live in `github.com/synqronlabs/mxraven-go/admin/api`.

The ergonomic layer covers every tenant-bound resource family, including
domains, listeners, API keys, routing rules, suppressions, recipient sets,
auto-reply templates, forwarding destinations, inbound routes, relays, storage
integrations, webhook endpoints, identity providers, quotas, rate limits, audit
log, resource search, abuse control, and mail analytics. The embedded generated
client remains available for escape hatches.

## Scope

The SDK covers tenant-bound administration only. Platform-administrator,
account self-service, billing, signup, and authentication operations are
intentionally excluded, along with global/platform suppressions, platform
resources, and platform commercial endpoints. The generated package currently
exposes 127 operations, including stored auto-reply template content at
`/v2/tenants/{slug}/auto-reply-templates/{template_id}/content` and the per-listener
client notification setting at
`/v2/tenants/{slug}/listeners/{listener_id}/notifications`.

## Authentication

Use `WithToken` for a static bearer token:

```go
admin.WithToken("your-access-token")
```

Use `WithTokenSource` when tokens expire and must be refreshed. The callback is
invoked for every request and receives the generated operation name, so one
source can serve multiple credentials:

```go
admin.WithTokenSource(func(ctx context.Context, operation string) (string, error) {
	token, err := tokenCache.Token(ctx)
	if err != nil {
		return "", err
	}
	return token, nil
})
```

A token source error aborts the request before it is sent.

## Handling errors

The API returns RFC 9457 problem details. The tenant-bound services convert
non-success responses into `*admin.APIError`, so callers branch on the stable
problem code with `errors.As`:

```go
domain, err := ws.Domains.Get(ctx, domainID)
if err != nil {
	var apiErr *admin.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case "not_found":
			return errNotFound
		case "conflict":
			return errAlreadyExists
		}
		log.Printf("trace=%s retry_after=%s", apiErr.TraceID, apiErr.RetryAfter)
	}
	return err
}
```

`IsCode` and `AsAPIError` are convenient shorthands:

```go
if admin.IsCode(err, "not_found") {
	// ...
}
```

For direct calls to the embedded generated client, `Do` and `Check` unwrap
generated results while preserving typed API errors:

```go
tenant, err := admin.Do[admin.Tenant](client.GetTenant(ctx, params))
err = admin.Check(client.DeleteDomain(ctx, params))
```

`ProblemFrom` and `TraceID` remain available for callers that need to inspect a
generated response value directly.

## Migrating from generated calls

The embedded generated client remains supported, but the tenant-bound services
remove response unions, repeated slugs, and per-call problem checks.

Before:

```go
res, err := client.GetDomain(ctx, api.GetDomainParams{Slug: "acme", DomainID: id})
if err != nil {
	return err
}
if problem, ok := admin.ProblemFrom(res); ok {
	return fmt.Errorf("get domain: %s", problem.Code)
}
domain := res.(*api.GetDomainOKHeaders).Response
```

After:

```go
domain, err := client.Workspace("acme").Domains.Get(ctx, id)
```

For direct generated-client calls, use `admin.Do` or `admin.Check` to retain
typed errors:

```go
domain, err := admin.Do[admin.Domain](client.GetDomain(ctx, params))
```

## Rate limits and retries

The client automatically retries responses the server rejected before
processing, so retries are safe even for non-idempotent operations:

- `429 Too Many Requests` is always retried.
- `503 Service Unavailable` is retried only when it carries `Retry-After`.
- Transport errors are never retried, because the request outcome is unknown.

`Retry-After` is honoured as delta seconds or an HTTP date and is capped by the
policy. When it is absent, the client uses capped exponential backoff.

The default policy is `admin.DefaultRetryPolicy` (four attempts). Override it
or disable retries:

```go
admin.WithRetry(admin.RetryPolicy{
	MaxAttempts: 5,
	BaseDelay:   250 * time.Millisecond,
	MaxDelay:    15 * time.Second,
})

// Or disable retries entirely:
admin.WithRetry(admin.RetryPolicy{MaxAttempts: 1})
```

## Pagination

List operations return lazy `iter.Seq2` sequences. Pages are fetched only as the
sequence is consumed, and stopping early stops further requests:

```go
for listener, err := range ws.Listeners.List(ctx, admin.ListListenersOptions{
	ListenerType: admin.ListenerTypeMta,
}) {
	if err != nil {
		return err
	}
	fmt.Println(listener.ID)
}
```

Use `Take` or `Collect` when a slice is more convenient:

```go
firstTen, err := admin.Take(ws.Listeners.List(ctx, admin.ListListenersOptions{}), 10)
all, err := admin.Collect(ws.Domains.List(ctx, admin.ListDomainsOptions{}))
```

`Take` fetches only the pages needed to satisfy the limit. Both helpers return
the items read so far together with any iteration error.

For explicit page control, use the generated `Pages` methods:

```go
pager := ws.Domains.Pages(admin.ListDomainsOptions{PageSize: 100})
for pager.Next(ctx) {
	for _, domain := range pager.Items() {
		fmt.Println(domain.DomainName)
	}
	page := pager.Page()
	log.Printf("previous=%q next=%q last=%q",
		page.PreviousToken, page.NextToken, page.LastToken)
}
if err := pager.Err(); err != nil {
	return err
}
```

Child-resource services use the same pattern:

```go
pager := ws.APIKeys.Pages(listenerID, admin.ListAPIKeysOptions{})
```

`Pager.Page`, `Pager.PreviousToken`, and `Pager.LastToken` expose the cursor
metadata of the most recently loaded page. `NextPageToken`,
`PreviousPageToken`, and `LastPageToken` remain available for raw generated
responses.

### List filters

List options use zero values for unset string and enum filters, and pointers for
tri-state booleans:

```go
verified := true
domains, err := admin.Collect(ws.Domains.List(ctx, admin.ListDomainsOptions{
	Search:      "example",
	Status:      admin.DomainStatusVerified,
	SPFVerified: &verified,
	PageSize:    100,
}))
```

Search terms must be 3–200 characters and page sizes 1–500; invalid options fail
locally before any request is sent.

## Listeners

Listener operations cover creation, renaming, routing defaults, Rspamd scanning,
and client notifications:

```go
listener, err := ws.Listeners.Create(ctx, admin.CreateListenerInput{
	DisplayName:                  "Outbound transactional",
	ListenerType:                 admin.ListenerTypeSubmission,
	StreamType:                   admin.CreatableListenerStreamTypeTransactional,
	DefaultTerminalActionType:    admin.DefaultTerminalActionTypeDELIVER,
	DefaultTerminalActionPayload: admin.DeliverPayload(),
})
if err != nil {
	return err
}

payload, err := admin.RejectPayload(550, "5.1.1", "No such user")
if err != nil {
	return err
}
listener, err = ws.Listeners.SetDefaultTerminalAction(
	ctx, listener.ID, admin.DefaultTerminalActionTypeREJECT, payload,
)
```

The payload helpers cover `DELIVER`, `DELIVER_DEDICATED`, `RELAY` and
`SMARTHOST_RELAY`, `AUTO_REPLY`, `DROP`, and `REJECT`. The control plane still
validates action-specific references and compatibility with the listener type.

## Resource services

Every tenant-bound family is reachable from `Workspace`. Child resources take
their parent identifier as an argument:

```go
keys, err := admin.Collect(ws.APIKeys.List(ctx, listenerID, admin.ListAPIKeysOptions{}))

key, err := ws.APIKeys.Create(ctx, listenerID)

rules, err := admin.Collect(ws.RoutingRules.List(ctx, listenerID, admin.ListRoutingRulesOptions{}))

members, err := admin.Collect(ws.RecipientSets.ListMembers(ctx, setRef, admin.ListRecipientSetMembersOptions{
	Search: "example.com",
}))
```

Analytics endpoints take a typed query struct instead of `Opt*` parameters:

```go
overview, err := ws.MailAnalytics.GetOverview(ctx, admin.GetMailAnalyticsOverviewQuery{
	StartAt: time.Now().Add(-24 * time.Hour),
	EndAt:   time.Now(),
})
```

Other generated services include `Suppressions`, `AutoReplyTemplates`,
`SMTPForwardDestinations`, `InboundRoutes`, `SMTPRelays`, `StorageIntegrations`,
`WebhookEndpoints`, `IdentityProviders`, `Quotas`, `MTARateLimits`, `AuditLog`,
`ResourceSearch`, `AbuseControl`, and `DedicatedIPPools`.

### Model aliases

The generated `admin` package re-exports the models and enums used by the facade,
so most programs need only one import:

```go
domain := admin.Domain{ /* ... */ }
status := admin.DomainStatusVerified
listenerType := admin.ListenerTypeSubmission
action := admin.DefaultTerminalActionTypeDELIVER
```

The full generated surface remains available in `admin/api` for models that are
not part of a facade signature.

## Options

| Option | Description |
| --- | --- |
| `WithBaseURL(string)` | Overrides the API base URL. Defaults to `admin.DefaultBaseURL`. |
| `WithToken(string)` | Static bearer token. Required unless a token source is set. |
| `WithTokenSource(func(ctx, operation) (string, error))` | Per-request token source. |
| `WithHTTPClient(*http.Client)` | Custom client for timeouts, proxies, and transports. |
| `WithUserAgent(string)` | Sets `User-Agent` when the request does not already have one. |
| `WithRetry(RetryPolicy)` | Overrides the retry policy. |

`Client` is safe for concurrent use by multiple goroutines.

## Development

`admin/api` is generated and must never be edited by hand. The authoritative
OpenAPI contract is proprietary and is not committed; copy it to
`openapi/v2/` and regenerate:

```sh
go generate ./admin
```

This runs `tools/openapifilter` to derive the tenant-only contract, invokes the
pinned ogen version to regenerate `admin/api`, then runs `tools/adminfacade` to
regenerate the ergonomic facade. See `admin/doc.go` and `admin/ogen.yaml`.

To verify committed facade files without rewriting them:

```sh
go run ./tools/adminfacade -api ./admin/api -openapi ./openapi/public/v2 -output ./admin -check
```
