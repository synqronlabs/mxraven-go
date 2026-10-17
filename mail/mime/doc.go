// Package mime parses inbound RFC 5322 / MIME messages into headers, addresses,
// text and HTML bodies, and decoded attachments.
//
// mxRaven webhook payloads describe inbound mail as JSON, and the full message
// remains available as raw RFC 5322 bytes behind the payload's RawEmail
// reference. [ParseEmail] turns those bytes into a structured [Email];
// [FetchAndParseRawEmail] downloads the referenced message with
// [github.com/synqronlabs/mxraven-go/mail/webhook.RawEmail.Fetch] and parses it
// in one step:
//
//	verifier, err := webhook.NewVerifier(webhook.WithSecret(signingSecret))
//	if err != nil {
//		return err
//	}
//	event, err := verifier.VerifyAndDecode(r)
//	if err != nil {
//		return err
//	}
//	if event.InboundEmail != nil {
//		email, err := mime.FetchAndParseRawEmail(r.Context(), event.InboundEmail.RawEmail, nil)
//		if err != nil {
//			return err
//		}
//		log.Printf("%s: %d attachments", email.Subject, len(email.Attachments))
//	}
//
// The parser is defensive: malformed messages yield the best available
// interpretation rather than an error. It understands multipart/mixed,
// multipart/alternative, and multipart/related, decodes base64 and
// quoted-printable transfer encodings, decodes RFC 2047 encoded words, and
// reassembles RFC 2231 parameters. message/rfc822 parts are surfaced as
// attachments rather than parsed recursively. Only input that is structurally
// unbounded — excessive nesting or an excessive number of parts — is rejected,
// with [ErrTooDeep] or [ErrTooManyParts].
package mime
