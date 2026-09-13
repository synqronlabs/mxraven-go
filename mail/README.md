# mxRaven Mail SDK

A Go SDK for the mxRaven mail-facing runtime surfaces: SMTP submission,
webhook verification and decoding, and recipient feedback.

The library wraps the open-source
[`github.com/synqronlabs/raven`](https://github.com/synqronlabs/raven) SMTP
client and message builder behind a small, mxRaven-specific API. Control-plane
administration such as suppression lists, deliverability reporting, and mail
analytics lives in the separate [`admin`](../admin) SDK.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/mail
```

Requires Go 1.26.4 or later (see `go.mod`).

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/synqronlabs/mxraven-go/mail"
)

func main() {
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
}
```

## Authentication

Submission authenticates with an mxRaven submission API key:

- **Username** is the key's username, for example `mxr_tx_ab12cd34ef56`. The
  stream code (`tx`, `mk`, `sys`) is part of the username.
- **Secret** is the key's secret, returned only when the key is created.

Create keys with the `admin` SDK or the control-plane API. The server requires
STARTTLS and SMTP AUTH before `MAIL FROM`; both are enabled by default and
cannot be disabled.

The `MAIL FROM` address and the visible `From` header must use a verified,
sending-enabled domain that is granted to the submission listener, otherwise
the server rejects the transaction with a permanent SMTP error.

## Composing messages

`Message` is a mutable, chainable builder. A message can be sent more than once;
it is serialized on each `Send`.

```go
msg := mail.NewMessage().
	From("Acme <noreply@acme.example>").
	ReplyTo("support@acme.example").
	To("customer@example.com").
	Cc("accounting@acme.example").
	Bcc("audit@acme.example").
	Subject("Invoice 1842").
	Header("X-Campaign", "september").
	Text("Your invoice is attached.").
	HTML("<p>Your invoice is attached.</p>").
	AttachFile("invoice.pdf", pdfBytes)
```

- Setting both `Text` and `HTML` produces a `multipart/alternative` body.
- `AttachFile` adds a file with an `application/octet-stream` content type;
  use `Attach` to set a content type, or `AttachInline` for `cid:` references.
- `Bcc` adds envelope recipients without a visible `Bcc` header.
- `NullSender` uses a null reverse-path for bounce and auto-generated mail while
  keeping the `From` header.

## Sending raw messages

Use `SendRaw` to stream an already serialized RFC 5322 message with an explicit
envelope. This avoids parsing and is the efficient path for large or
pre-rendered messages. The caller is responsible for RFC 5322 correctness.

```go
result, err := client.SendRaw(ctx,
	mail.Envelope{
		From: "bounce@acme.example",
		To:   []string{"customer@example.com"},
	},
	renderedReader,
)
```

## Results

`Send` returns a `*Result`:

| Field | Description |
| --- | --- |
| `MessageRef` | mxRaven message reference parsed from the `250 2.0.0 accepted; message_ref=<uuid>` reply. |
| `Code`, `Message` | The final SMTP reply. `Message` is for humans and is not stable. |
| `Recipients` | Per-recipient acceptance, when the server returned individual `RCPT` responses. |

`MessageRef` is the durable reference to use for correlation and analytics.

## Errors

Rejected commands are returned as `*mail.SMTPError`:

```go
var smtpErr *mail.SMTPError
if errors.As(err, &smtpErr) {
	switch {
	case smtpErr.Permanent():
		// 5xx: do not retry the same message.
	case smtpErr.Transient():
		// 4xx: retry later.
	}
}
```

When the server replied but rejected the transaction, `Send` still returns a
`*Result` alongside the error so per-recipient detail is available.

Transport failures, connection errors, and context cancellation are returned
unchanged as wrapped errors.

## Connection pooling

`Client` keeps a bounded pool of authenticated connections and is safe for
concurrent use. The pool size defaults to 5 and is set with `WithPoolSize`.
A connection whose command fails is discarded rather than reused, because a
failed command can leave unread replies on the wire.

Always call `Close` to release pooled connections.

## Options

| Option | Description |
| --- | --- |
| `WithAddress(host string, port int)` | Required. Submission server host and port. |
| `WithCredentials(username, secret string)` | Required. Submission API key. |
| `WithTLSConfig(*tls.Config)` | STARTTLS configuration. The default requires TLS 1.2+ and verifies the server certificate. |
| `WithPoolSize(size int)` | Maximum pooled connections. Default 5. |
| `WithConnectTimeout(timeout time.Duration)` | TCP connect timeout. Default 30s. |
| `WithLocalName(name string)` | EHLO name. Default `localhost`. |

`DefaultAddressPort` is the mxRaven submission port (587).

## Subpackages

- [`mail/webhook`](./webhook/README.md) verifies and decodes `DELIVER_WEBHOOK`
  and `NOTIFY_WEBHOOK` deliveries, and downloads the raw message referenced by
  an inbound-email payload.
- [`mail/feedback`](./feedback/README.md) submits tenant spam/ham training and
  performs RFC 8058 one-click unsubscribes.

## Server limits

The submission service enforces these limits; exceeding them returns an SMTP
error or truncates the transaction:

- 25 MB maximum message size (advertised as `SIZE 26214400`).
- 100 envelope recipients per transaction, counting `Bcc` and recipients added
  by routing rules.
- Three `Received` headers retained.

Reserved headers (`DKIM-Signature`, `Return-Path`, `Authentication-Results`,
`Received-SPF`, ARC headers, and common spam/virus headers) are stripped by the
server and cannot be injected by the client. Only the first client `Received`
header is preserved.

## Development

```sh
go test ./mail/...
golangci-lint run ./mail/...
```
