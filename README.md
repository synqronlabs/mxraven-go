# mxRaven Go SDK

The official Go SDK for [mxRaven](https://mxraven.com). It
covers both sides of an integration:

- **Control plane:** manage tenants, listeners, domains, routing rules,
  webhook endpoints, suppressions, and analytics.
- **Mail runtime:** submit mail over SMTP, receive and verify webhooks, parse
  inbound messages, send recipient feedback, and consume client notification
  events over NATS.

## Packages

| Package | Import path | Purpose |
| --- | --- | --- |
| `admin` | `github.com/synqronlabs/mxraven-go/admin` | Client for the control-plane v2 HTTP API. |
| `admin/api` | `github.com/synqronlabs/mxraven-go/admin/api` | Generated transport for the tenant-bound contract. |
| `mail` | `github.com/synqronlabs/mxraven-go/mail` | SMTP submission. |
| `mail/webhook` | `github.com/synqronlabs/mxraven-go/mail/webhook` | Webhook verification and decoding. |
| `mail/feedback` | `github.com/synqronlabs/mxraven-go/mail/feedback` | Spam/ham learning and one-click unsubscribe. |
| `mail/notify` | `github.com/synqronlabs/mxraven-go/mail/notify` | Client notification stream over NATS. |
| `mail/mime` | `github.com/synqronlabs/mxraven-go/mail/mime` | Inbound RFC 5322 / MIME parsing. |

## Requirements

Go 1.26.4 or later (see [`go.mod`](./go.mod)).

## Install

```sh
go get github.com/synqronlabs/mxraven-go
```

## Control plane (`admin`)

`admin` is a client for the mxRaven control-plane HTTP API. It embeds the
generated [`admin/api`](./admin/api) client, so every operation described by the
authoritative OpenAPI contract is available directly, plus cross-cutting
helpers for authentication, retries, RFC 9457 problem extraction, and
pagination.

The generated transport is produced from a **tenant-only** subset of the
contract. Platform-administrator, account self-service, billing, signup,
authentication, and global/platform suppression operations are intentionally
excluded.

```go
import (
	"context"
	"fmt"
	"log"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

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
```

### Authentication

Use `WithToken` for a static bearer token, or `WithTokenSource` when tokens
expire and must be refreshed. The source is invoked for every request and
receives the generated operation name, so a single source can serve multiple
credentials. A token source error aborts the request before it is sent.

```go
admin.WithTokenSource(func(ctx context.Context, operation string) (string, error) {
	return tokenCache.Token(ctx)
})
```

### Problems and tracing

The contract declares every expected error response, so the generated client
decodes non-2xx responses as typed values rather than Go errors. `ProblemFrom`
extracts the RFC 9457 problem document from any response value; branch on
`Code`, never on `Title` or `Detail`. `TraceID` reads the `X-Trace-ID` header.

```go
res, err := client.CreateDomain(ctx, req, params)
if err != nil {
	return err
}
if problem, ok := admin.ProblemFrom(res); ok {
	switch problem.Code {
	case "conflict":
		return errAlreadyExists
	default:
		return fmt.Errorf("create domain: %s: %s", problem.Code, problem.Title)
	}
}
if trace, ok := admin.TraceID(res); ok {
	log.Printf("trace %s", trace)
}
```

### Rate limits and retries

The client automatically retries responses the server rejected before
processing, so retries are safe even for non-idempotent operations:

- `429 Too Many Requests` is always retried.
- `503 Service Unavailable` is retried only when it carries `Retry-After`.
- Transport errors are never retried, because the request outcome is unknown.

`Retry-After` is honoured as delta seconds or an HTTP date and is capped by the
policy. The default is `admin.DefaultRetryPolicy` (four attempts); override or
disable it with `admin.WithRetry`.

```go
admin.WithRetry(admin.RetryPolicy{
	MaxAttempts: 5,
	BaseDelay:   250 * time.Millisecond,
	MaxDelay:    15 * time.Second,
})

// Disable retries entirely:
admin.WithRetry(admin.RetryPolicy{MaxAttempts: 1})
```

### Pagination

List operations return a page and an opaque `X-Next-Page-Token`. Use `Pager` to
iterate lazily without risking rate limits; stop at any point and no further
pages are requested. `NextPageToken`, `PreviousPageToken`, and `LastPageToken`
read the corresponding headers for one-off paging.

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

See the [`admin` README](./admin/README.md) for the full option list.

## Mail submission (`mail`)

`mail` submits mail to the mxRaven SMTP submission service. It wraps the
open-source [`github.com/synqronlabs/raven`](https://github.com/synqronlabs/raven)
client and message builder behind an ergonomic, mxRaven-specific API.

Authentication uses a submission **API key**: the username is the key's
username (for example `mxr_tx_ab12cd34ef56`) and the secret is the key's
secret. STARTTLS and SMTP AUTH are required and enabled by default. The
`MAIL FROM` address and `From` header must use a verified, sending-enabled
domain that is granted to the listener.

```go
import (
	"context"
	"fmt"
	"log"

	"github.com/synqronlabs/mxraven-go/mail"
)

client, err := mail.New(
	mail.WithAddress("smtp.mxraven.com", mail.DefaultAddressPort),
	mail.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
)
if err != nil {
	log.Fatal(err)
}
defer client.Close()

result, err := client.Send(context.Background(), mail.NewMessage().
	From("Acme <noreply@acme.example>").
	To("customer@example.com").
	Subject("Your receipt").
	Text("Thanks for your order.").
	HTML("<p>Thanks for your order.</p>"))
if err != nil {
	log.Fatal(err)
}

fmt.Println("accepted as", result.MessageRef)
```

See the [`mail` README](./mail/README.md) for message composition, raw sending,
limits, and options.

## Webhooks (`mail/webhook`)

`mail/webhook` verifies and decodes the two mxRaven webhook actions,
`DELIVER_WEBHOOK` (a complete inbound message) and `NOTIFY_WEBHOOK` (delivery
status from the SMTP or object-storage workers). Every delivery is signed with
HMAC-SHA256 over a canonical request string and the signature headers
`X-MxRaven-*`.

```go
import (
	"net/http"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

verifier, err := webhook.NewVerifier(webhook.WithSecret(signingSecret))
if err != nil {
	log.Fatal(err)
}

http.HandleFunc("/mxraven/webhook", func(w http.ResponseWriter, r *http.Request) {
	event, err := verifier.VerifyAndDecode(r)
	if err != nil {
		http.Error(w, "invalid webhook", http.StatusUnauthorized)
		return
	}

	switch {
	case event.InboundEmail != nil:
		// DELIVER_WEBHOOK: full inbound message.
		raw, err := event.InboundEmail.RawEmail.Fetch(r.Context(), nil)
		if err != nil {
			http.Error(w, "fetch failed", http.StatusBadGateway)
			return
		}
		_ = raw
	case event.DeliveryStatus != nil:
		// NOTIFY_WEBHOOK: SMTP delivery status.
	case event.StorageStatus != nil:
		// NOTIFY_WEBHOOK: object-storage status.
	}
	w.WriteHeader(http.StatusNoContent)
})
```

The signing secret is shown only once, when the endpoint is created or its
secret is rotated. See the [`webhook` README](./mail/webhook/README.md) for the
signature scheme, payload examples, and delivery semantics.

## Inbound parsing (`mail/mime`)

An inbound-email webhook carries a `raw_email` reference rather than the message
bytes. `mail/mime` downloads and parses it into headers, addresses, text and
HTML bodies, and decoded attachments:

```go
import (
	"context"

	"github.com/synqronlabs/mxraven-go/mail/mime"
)

if event.InboundEmail != nil {
	email, err := mime.FetchAndParseRawEmail(context.Background(), event.InboundEmail.RawEmail, nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, attachment := range email.Attachments {
		log.Printf("%s (%s), inline=%t", attachment.Filename, attachment.ContentType, attachment.Inline)
	}
}
```

`mime.ParseEmail` parses bytes you already hold. The parser handles
`multipart/mixed`, `multipart/alternative`, and `multipart/related`, decodes
`base64` and `quoted-printable` transfer encodings, decodes RFC 2047 encoded
words and RFC 2231 parameters, and falls back to UTF-8 and then Windows-1252 for
unknown charsets. Inline images are reported as attachments with `Inline` set
and their `ContentID`.

See the [`mime` README](./mail/mime/README.md) for the parsed-message fields and
parser limits.

## Feedback (`mail/feedback`)

`mail/feedback` submits recipient feedback to the mxRaven feedback service.
Tenants teach the spam filter by reporting misclassified messages; the service
matches the submitted bytes to its stored evidence by SHA-256.

```go
import (
	"bytes"
	"context"
	"log"

	"github.com/synqronlabs/mxraven-go/mail/feedback"
)

client, err := feedback.New(
	feedback.WithBaseURL("https://feedback.mxraven.com"),
	feedback.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
)
if err != nil {
	log.Fatal(err)
}

result, err := client.LearnSpam(context.Background(), bytes.NewReader(rawMessage))
if err != nil {
	log.Fatal(err)
}
log.Printf("learned as %s", result.Disposition)
```

Learning uses the same submission API key as SMTP submission and accepts
`LearnSpam`/`LearnHam` (or `Learn` with a disposition). The package also exposes
`Unsubscribe`, which performs the RFC 8058 one-click unsubscribe that recipient
mail clients call. Non-success responses are `*feedback.Error` with
`StatusCode` and `Retryable`.

Suppression lists, deliverability reporting, and mail analytics remain in the
`admin` SDK.

See the [`feedback` README](./mail/feedback/README.md) for limits and error
codes.

## Client notifications (`mail/notify`)

`mail/notify` consumes the durable mail events a listener publishes when client
notifications are enabled. Events are read from the listener's own JetStream
consumer over a NATS WebSocket connection, authenticated with the same
submission API key.

```go
import (
	"context"
	"log"

	"github.com/synqronlabs/mxraven-go/mail/notify"
)

client, err := notify.New(
	notify.WithURL("wss://nats.mxraven.com"),
	notify.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
	notify.WithListener(listenerID),
)
if err != nil {
	log.Fatal(err)
}
defer client.Close()

err = client.Consume(context.Background(), func(ctx context.Context, event notify.Event) error {
	switch {
	case event.Accepted != nil:
		log.Printf("accepted %s", event.Accepted.TaskID)
	case event.Attempt != nil:
		log.Printf("attempt %s: %s", event.Attempt.TaskID, event.Attempt.Status)
	}
	return nil
})
if err != nil {
	log.Fatal(err)
}
```

An accepted event reuses the webhook payload model, including raw-message access
for MTA listeners. `Consume` acknowledges an event when its handler returns nil
and retries it with backoff when the handler returns an error;
`Client.Events` is an iterator for callers that acknowledge explicitly.

Notifications are off by default and are enabled per listener with the `admin`
SDK. See the [`notify` README](./mail/notify/README.md) for the event model,
retention, and options.

## Repository layout

```text
admin/                Control-plane SDK
  api/                Generated ogen transport (do not edit)
  *.go                Client, retries, problems, pagination
mail/                 SMTP submission SDK
  webhook/            Webhook verification and decoding
  feedback/           Spam/ham learning and one-click unsubscribe
  notify/             Client notification stream over NATS
  mime/               Inbound RFC 5322 / MIME parsing
tools/openapifilter/  Derives the tenant-only OpenAPI contract for generation
```

`admin/api` is generated and must not be edited by hand. The authoritative
OpenAPI contract is proprietary and is not committed; copy it to `openapi/v2/`
and regenerate with `go generate ./admin`. See [`admin/doc.go`](./admin/doc.go).

## Development

```sh
gofmt -l .
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```

## License

Apache License 2.0. See [`LICENSE`](./LICENSE) and [`NOTICE`](./NOTICE).
