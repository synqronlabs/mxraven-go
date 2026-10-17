# mxRaven MIME Parsing SDK

Inbound message parsing for the mxRaven mail runtime. It turns the raw RFC 5322
bytes referenced by an inbound-email webhook payload into headers, addresses,
text and HTML bodies, and decoded attachments.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/mail/mime
```

Requires Go 1.26.4 or later (see `go.mod`).

## Parsing a message

`ParseEmail` parses bytes you already hold:

```go
import (
	"fmt"

	"github.com/synqronlabs/mxraven-go/mail/mime"
)

email, err := mime.ParseEmail(rawBytes)
if err != nil {
	return err
}
fmt.Println(email.Subject, email.From, len(email.Attachments))
```

An inbound-email webhook payload carries a `raw_email` reference rather than the
message bytes. `FetchAndParseRawEmail` downloads, verifies, and parses it in one
step using the webhook package's raw-email client:

```go
import (
	"context"

	"github.com/synqronlabs/mxraven-go/mail/mime"
)

event, err := verifier.VerifyAndDecode(r)
if err != nil {
	return err
}
if event.InboundEmail != nil {
	email, err := mime.FetchAndParseRawEmail(r.Context(), event.InboundEmail.RawEmail, nil)
	if err != nil {
		return err
	}
	for _, attachment := range email.Attachments {
		fmt.Println(attachment.Filename, attachment.ContentType, len(attachment.Content))
	}
}
```

The download verifies the declared size and SHA-256 digest. The payload's
short-lived bearer token is sent in the `Authorization` header and must not be
logged. A nil `*http.Client` uses the fetch default.

## Parsed message

| Field | Description |
| --- | --- |
| `Headers` | Every header field, in order, with continuation lines unfolded. Values are not decoded. |
| `Subject` | The decoded `Subject`. |
| `From`, `To`, `Cc`, `Bcc`, `ReplyTo` | Parsed mailboxes with decoded display names. |
| `MessageID` | The `Message-ID` without angle brackets. |
| `Date` | The parsed `Date`, when present and parseable. |
| `Text`, `HTML` | The first decoded `text/plain` and `text/html` bodies. |
| `Attachments` | Every non-body part with decoded content, filename, content type, and Content-ID. |

Zero values mean the header or body was absent. Attachment content is a fresh
copy the caller owns.

`Attachment.Inline` is true when a part is marked `inline` or carries a
`Content-ID`, which is how an inline image referenced by `cid:` from an HTML
body is identified:

```go
for _, attachment := range email.Attachments {
	if attachment.Inline {
		fmt.Println("inline", attachment.ContentID, attachment.ContentType)
	}
}
```

## Parser behavior

The parser is defensive: malformed messages yield the best available
interpretation instead of an error. It understands `multipart/mixed`,
`multipart/alternative`, and `multipart/related`, decodes `base64` and
`quoted-printable` transfer encodings, decodes RFC 2047 encoded words, and
reassembles RFC 2231 parameters. Declared charsets are decoded; unknown charsets
fall back to UTF-8 and then Windows-1252. `message/rfc822` parts are surfaced as
attachments rather than parsed recursively.

Only structurally unbounded input is rejected, so an untrusted message cannot
exhaust the stack or memory:

| Error | Meaning |
| --- | --- |
| `mime.ErrTooDeep` | MIME nesting exceeds 20 levels. |
| `mime.ErrTooManyParts` | More than 2000 MIME entities. |

## Development

```sh
go test ./mail/mime/...
golangci-lint run ./mail/mime/...
```
