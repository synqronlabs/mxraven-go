# mxRaven Admin SDK

A Go client for the mxRaven control-plane v2 HTTP API.

The SDK exposes tenant-scoped administration operations generated from the
authoritative OpenAPI contract, plus a thin, hand-written facade for client
construction, bearer authentication, rate-limit handling, RFC 9457 problem
extraction, and pagination.

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
	"github.com/synqronlabs/mxraven-go/admin/api"
)

func main() {
	client, err := admin.New(
		admin.WithBaseURL("https://api.mxraven.com"),
		admin.WithToken("your-access-token"),
	)
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		log.Fatal(err) // transport or decode failure
	}
	if problem, ok := admin.ProblemFrom(res); ok {
		log.Fatalf("get tenant: %s: %s", problem.Code, problem.Title)
	}

	tenant := res.(*api.TenantHeaders).Response
	fmt.Println(tenant.Slug, tenant.Status)
}
```

`Client` embeds the generated `*api.Client`, so every generated operation is a
method on the SDK client. The `admin` package adds only cross-cutting helpers;
the generated types and operations live in
`github.com/synqronlabs/mxraven-go/admin/api`.

## Scope

The SDK covers tenant-bound administration only. Platform-administrator,
account self-service, billing, signup, and authentication operations are
intentionally excluded, along with global/platform suppressions, platform
resources, and platform commercial endpoints. The generated package currently
exposes 123 operations.

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

The API returns RFC 9457 problem details. Because the contract declares every
expected error response, the generated client decodes non-2xx responses as
typed values rather than Go errors. `ProblemFrom` extracts the problem document
from any response value:

```go
res, err := client.CreateDomain(ctx, req, params)
if err != nil {
	return err
}
if problem, ok := admin.ProblemFrom(res); ok {
	// Branch on Code. Do not parse Title or Detail.
	switch problem.Code {
	case "conflict":
		return errAlreadyExists
	default:
		return fmt.Errorf("create domain: %s: %s", problem.Code, problem.Title)
	}
}
```

`TraceID` returns the `X-Trace-ID` header value when present:

```go
if trace, ok := admin.TraceID(res); ok {
	log.Printf("trace %s", trace)
}
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

List operations return a page and an opaque `X-Next-Page-Token`. Use the lazy
`Pager` to iterate without eagerly fetching every page, which would risk rate
limits:

```go
fetch := func(ctx context.Context, token string) (admin.Page[api.Listener], error) {
	params := api.ListListenersParams{Slug: "acme"}
	if token != "" {
		params.PageToken = api.OptString{Value: token, Set: true}
	}

	res, err := client.ListListeners(ctx, params)
	if err != nil {
		return admin.Page[api.Listener]{}, err
	}
	if problem, ok := admin.ProblemFrom(res); ok {
		return admin.Page[api.Listener]{}, fmt.Errorf("list listeners: %s", problem.Code)
	}

	page := res.(*api.ListListenersOKHeaders)
	next, _ := admin.NextPageToken(res)
	return admin.Page[api.Listener]{Items: page.Response, NextToken: next}, nil
}

pager, err := admin.NewPager(fetch)
if err != nil {
	return err
}
for pager.Next(ctx) {
	for _, listener := range pager.Items() {
		fmt.Println(listener.ID)
	}
}
if err := pager.Err(); err != nil {
	return err
}
```

Stop iterating at any point; no further pages are requested. For one-off paging,
`NextPageToken`, `PreviousPageToken`, and `LastPageToken` read the corresponding
response headers.

## Options

| Option | Description |
| --- | --- |
| `WithBaseURL(string)` | Required. API base URL, e.g. `https://api.mxraven.com`. |
| `WithToken(string)` | Static bearer token. |
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

This runs `tools/openapifilter` to derive the tenant-only contract, then invokes
the pinned ogen version. See `admin/doc.go` and `admin/ogen.yaml`.
