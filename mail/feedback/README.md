# mxRaven Feedback SDK

Submit recipient feedback to the mxRaven feedback service.

Tenants can teach the mxRaven spam filter by reporting messages that were
misclassified. The service matches a submitted message to the exact bytes it
processed, so the raw RFC 822 message must be provided unchanged. This package
also exposes the RFC 8058 one-click unsubscribe endpoint that recipient mail
clients call.

Suppression-list management, deliverability reporting, and mail analytics are
not part of this package; use the [`admin`](../../admin) SDK for those.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/mail/feedback
```

## Quick start

```go
package main

import (
	"bytes"
	"context"
	"log"

	"github.com/synqronlabs/mxraven-go/mail/feedback"
)

func main() {
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
	log.Printf("learned as %s (matched %s)", result.Disposition, result.MatchedHashKind)
}
```

## Authentication

Learning requests authenticate with a submission API key using HTTP Basic auth:

- **Username** is the key's username, for example `mxr_tx_ab12cd34ef56`.
- **Secret** is the key's secret.

Credentials are set with `WithCredentials`. The one-click unsubscribe endpoint
is unauthenticated and does not require them.

## Learning

`Learn` submits one training example. `LearnSpam` and `LearnHam` are
convenience wrappers.

```go
result, err := client.Learn(ctx, feedback.DispositionHam, bytes.NewReader(rawMessage))
```

`Disposition` is `feedback.DispositionSpam` (`"spam"`) or
`feedback.DispositionHam` (`"ham"`).

The service identifies the message by its SHA-256 hash:

- `matched_hash_kind` is `rendered_eml_sha256` when the rendered message matched,
  otherwise `accepted_eml_sha256` for the accepted message.
- If neither hash matches stored evidence, the request fails with a `404`.
  Submit the exact bytes mxRaven processed, including original line endings.
- Submitting a message with no stored evidence cannot be learned.

The maximum submitted size is the service's `max_report_bytes` setting (1 MiB by
default). Larger bodies fail with a `413`.

On success `LearningResult` reports:

| Field | Description |
| --- | --- |
| `Status` | Normally `"learned"`. |
| `Disposition` | The applied training label. |
| `TenantID` | Tenant that owns the matched message. |
| `ListenerID` | Listener that processed the matched message. |
| `MatchedHashKind` | Which stored hash matched. |

The service applies per-tenant rate and concurrency limits and returns `429`
when they are exceeded; `Error.Retryable` reports this.

## One-click unsubscribe

`Unsubscribe` performs an RFC 8058 one-click unsubscribe for a token. This is
the operation a recipient mail client performs against the `List-Unsubscribe`
URL; applications rarely call it directly.

```go
if err := client.Unsubscribe(ctx, token); err != nil {
	return err
}
```

mxRaven adds `List-Unsubscribe` and `List-Unsubscribe-Post` headers to marketing
stream messages automatically; the token is signed server-side. The method
posts `List-Unsubscribe=One-Click` to
`/v1/feedback/unsubscribe/{token}`.

## Errors

Non-success responses are returned as `*feedback.Error`:

```go
var feedbackErr *feedback.Error
if errors.As(err, &feedbackErr) {
	if feedbackErr.Retryable() {
		// 429 or 5xx: retry later.
	}
}
```

`Retryable` reports true for `429 Too Many Requests` and `5xx`. Branch on
`StatusCode`, not on the human-readable `Message`:

| Status | Meaning |
| --- | --- |
| `400` | Empty body or invalid disposition. |
| `401` | Missing or invalid credentials. |
| `404` | No matching message evidence. |
| `413` | Raw MIME exceeds the service limit. |
| `429` | Learning rate or concurrency limit exceeded. |
| `502` | The learning backend failed. |
| `503` | Learning is not configured on the server. |

## Options

| Option | Description |
| --- | --- |
| `WithBaseURL(baseURL string)` | Required. Feedback service base URL. |
| `WithCredentials(username, secret string)` | API key used by the learning methods. |
| `WithHTTPClient(*http.Client)` | Custom client. The default has a 30-second timeout. |

`Client` is safe for concurrent use.

## Development

```sh
go test ./mail/feedback/...
golangci-lint run ./mail/feedback/...
```
