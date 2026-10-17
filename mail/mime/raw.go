package mime

import (
	"context"
	"fmt"
	"net/http"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

// FetchAndParseRawEmail downloads the raw message referenced by an inbound-email
// payload and parses it.
//
// The download verifies the declared size and SHA-256 digest and is bounded
// exactly like [github.com/synqronlabs/mxraven-go/mail/webhook.RawEmail.Fetch];
// the payload's short-lived bearer token is sent in the Authorization header
// and must not be logged. A nil client uses the fetch default.
func FetchAndParseRawEmail(ctx context.Context, raw webhook.RawEmail, client *http.Client) (*Email, error) {
	data, err := raw.Fetch(ctx, client)
	if err != nil {
		return nil, err
	}
	email, err := ParseEmail(data)
	if err != nil {
		return nil, fmt.Errorf("mime: parse fetched message: %w", err)
	}
	return email, nil
}
