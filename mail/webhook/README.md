# mxRaven Webhook SDK

Verify and decode mxRaven webhook deliveries.

mxRaven signs every webhook delivery with HMAC-SHA256 over a canonical request
string and sends the signature in `X-MxRaven-*` headers. This package verifies
that signature and decodes the JSON body into typed values. It handles both
webhook actions:

- **`DELIVER_WEBHOOK`** — a durable delivery carrying a complete inbound
  message (`event_type: "inbound_email"`).
- **`NOTIFY_WEBHOOK`** — a best-effort delivery-status callback from the SMTP
  or object-storage workers.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/mail/webhook
```

## Quick start

```go
package main

import (
	"log"
	"net/http"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

func main() {
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
			handleInbound(*event.InboundEmail)
		case event.DeliveryStatus != nil:
			handleStatus(*event.DeliveryStatus)
		case event.StorageStatus != nil:
			handleStorage(*event.StorageStatus)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

The signing secret is shown only once, when the webhook endpoint is created or
its secret is rotated. Store it securely and pass it to `WithSecret`. The secret
is used as literal key bytes; **do not base64-decode it**.

## Verification

`Verify` reads, restores, and checks the request body, so callers can decode it
afterwards. `VerifyAndDecode` does both in one step.

The signature is HMAC-SHA256 over this canonical string, with fields joined by
newlines (`\n`):

```text
{timestamp}
{webhook_id}
{METHOD}
{lowercase host, including port when present}
{escaped path plus raw query, or "/"}
{lowercase hex SHA-256 of the exact raw body}
```

| Header | Meaning |
| --- | --- |
| `X-MxRaven-Webhook-ID` | The delivery (task) ID. Stable across retries. |
| `X-MxRaven-Timestamp` | Unix signing time, in seconds. |
| `X-MxRaven-Signature` | `sha256=<lowercase hex HMAC>`. |
| `X-MxRaven-Signature-Kid` | Signing key ID. |

The timestamp is checked against the current time with a 5-minute tolerance by
default. Disable the check with `WithTolerance(0)`. Sign the raw bytes exactly
as received: re-serializing JSON changes the body hash and breaks verification.

### Key rotation

Pass the current secret with `WithSecret`, or register per-key secrets with
`WithKey` when more than one key is valid at once:

```go
verifier, err := webhook.NewVerifier(
	webhook.WithKey("whk_current", currentSecret),
	webhook.WithKey("whk_previous", previousSecret),
)
```

When key-specific secrets are configured, the `X-MxRaven-Signature-Kid` header
must name one of them.

## Decoding

`Decode` dispatches on the payload's `event_type`. SMTP delivery statuses carry
no `event_type`, so a body with a `status` field and no recognized event type is
decoded as a `DeliveryStatus`.

```go
switch {
case event.InboundEmail != nil:
	// event.Type == webhook.EventTypeInboundEmail
case event.DeliveryStatus != nil:
	// event.Type == webhook.EventTypeDeliveryStatus
case event.StorageStatus != nil:
	// event.Type == webhook.EventTypeStorageStatus
}
```

### `DELIVER_WEBHOOK` — `InboundEmail`

Carries the envelope, a parsed header summary, every header, spam/malware
verdicts, the routing decision, and a time-limited handle to the raw message.

```json
{
  "event_type": "inbound_email",
  "task_id": "33b97c39-6df9-45ce-b07b-83d81fa929bc",
  "tenant_id": "f714c301-58d1-45fa-b968-c5132f13e228",
  "listener_id": "98faf38a-5272-42fd-8b5b-85dbab818486",
  "attempt": 1,
  "accepted_at_utc": 1789302600,
  "occurred_at_utc": 1789302601,
  "routing_decision": {
    "terminal_action": "TERMINAL_ACTION_TYPE_RELAY",
    "matched_rule_id": "0d998525-a82d-48af-a01d-a534aa0bf920",
    "used_listener_default": false
  },
  "verdicts": {
    "action": "add header",
    "score": 7.5,
    "required_score": 6,
    "is_spam": true,
    "has_malware": false,
    "malware_names": [],
    "is_skipped": false,
    "error": ""
  },
  "envelope": {
    "mail_from": "sender@example.net",
    "rcpt_to": ["support@example.com"]
  },
  "message": {
    "subject": "Question about order 1842",
    "from": ["Customer <sender@example.net>"],
    "to": ["Support <support@example.com>"],
    "message_id": "<20260913.1842@example.net>",
    "date": "Sun, 13 Sep 2026 14:30:00 +0000"
  },
  "headers": [
    {"name": "From", "value": "Customer <sender@example.net>"},
    {"name": "To", "value": "Support <support@example.com>"}
  ],
  "raw_email": {
    "url": "https://raw.example.com/messages/33b97c39-6df9-45ce-b07b-83d81fa929bc.eml",
    "token_type": "Bearer",
    "access_token": "<short-lived token>",
    "expires_at_utc": 1789303501,
    "size_bytes": 18342,
    "sha256_hex": "7e54d2...1e46",
    "content_type": "multipart/alternative"
  }
}
```

`routing_decision.terminal_action` is the mxRaven worker enum string, for
example `TERMINAL_ACTION_TYPE_RELAY`. Use the exported `TerminalAction`
constants instead of comparing literals.

### `NOTIFY_WEBHOOK` — `DeliveryStatus` (SMTP)

Sent by the SMTP egress worker when a listener attaches a `NOTIFY_WEBHOOK`
action. It has no `event_type` field.

```json
{
  "task_id": "33b97c39-6df9-45ce-b07b-83d81fa929bc",
  "tenant_id": "f714c301-58d1-45fa-b968-c5132f13e228",
  "listener_id": "98faf38a-5272-42fd-8b5b-85dbab818486",
  "status": "deferred",
  "attempt": 2,
  "accepted_at_utc": 1789302600,
  "occurred_at_utc": 1789302700,
  "destination_domain": "example.com",
  "remote_host": "mx.example.com",
  "smtp_code": 451,
  "enhanced_status_code": "4.7.1",
  "remote_response": "greylisted, try again later",
  "next_retry_at_utc": 1789303000
}
```

### `NOTIFY_WEBHOOK` — `StorageStatus` (object storage)

Sent by the object-storage egress worker and identified by
`event_type: "s3_egress_status"`.

```json
{
  "event_type": "s3_egress_status",
  "task_id": "33b97c39-6df9-45ce-b07b-83d81fa929bc",
  "tenant_id": "f714c301-58d1-45fa-b968-c5132f13e228",
  "listener_id": "98faf38a-5272-42fd-8b5b-85dbab818486",
  "status": "delivered",
  "attempt": 1,
  "occurred_at_utc": 1789302700,
  "storage_ref": "archive",
  "bucket_name": "mail-archive",
  "object_key": "2026/09/13/33b97c39-6df9-45ce-b07b-83d81fa929bc.eml"
}
```

`StatusOutcome` is one of `attempted`, `delivered`, `deferred`, `failed`,
`expired`, or `suppressed` (SMTP only).

## Downloading the raw message

`RawEmail.Fetch` downloads the raw RFC 822 message for a `DELIVER_WEBHOOK`
payload. It sends the payload's short-lived bearer token and verifies the
downloaded bytes against the declared size and SHA-256 digest.

```go
raw, err := event.InboundEmail.RawEmail.Fetch(ctx, nil)
if err != nil {
	return err
}
// raw is the exact message bytes; its SHA-256 matches raw_email.sha256_hex.
```

Pass a custom `*http.Client` for proxies or custom timeouts; `nil` uses a
client with a 60-second timeout. The token is short-lived (15 minutes by
default); fetch the message promptly after receiving the webhook.

Treat `raw_email.access_token` as a secret. Do not log it.

## Delivery semantics

- `DELIVER_WEBHOOK` is delivered at least once. `task_id` is deterministic and
  stable across retries; deduplicate on it. `attempt` starts at 1.
- `NOTIFY_WEBHOOK` callbacks are best-effort, are not retried, and are not
  ordered. A single `task_id` can produce many statuses. Deduplicate on
  `task_id` plus `status`, `attempt`, and `occurred_at_utc`.
- `X-MxRaven-Webhook-ID` is the task ID, not a unique per-attempt identifier; it
  is shared by retries and by status callbacks for the same task.
- Return a 2xx response to acknowledge a delivery. The sender treats `429` and
  `5xx` as retryable and other non-2xx responses as permanent failures.

## Options

| Option | Description |
| --- | --- |
| `WithSecret(secret string)` | Signing secret, used as literal bytes. |
| `WithKey(kid, secret string)` | Per-key secret. Repeat to accept multiple keys. |
| `WithTolerance(d time.Duration)` | Maximum clock skew. Default 5m; `0` disables the timestamp check. |
| `WithMaxBodyBytes(n int64)` | Maximum body size read by `Verify`. Default 1 MiB. |

`ErrInvalidSignature` is returned when the HMAC does not match. Missing or
malformed headers and oversized bodies return descriptive errors.

## Development

```sh
go test ./mail/webhook/...
golangci-lint run ./mail/webhook/...
```
