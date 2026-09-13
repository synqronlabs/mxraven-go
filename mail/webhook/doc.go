// Package webhook verifies and decodes mxRaven webhook deliveries.
//
// mxRaven signs every webhook delivery with HMAC-SHA256 over a canonical
// request string and sends the signature in the X-MxRaven-* headers. A server
// that exposes a webhook endpoint verifies the request with a [Verifier] and
// decodes the JSON body with [Decode]:
//
//	verifier, err := webhook.NewVerifier(webhook.WithSecret(signingSecret))
//	if err != nil {
//		return err
//	}
//
//	func handle(w http.ResponseWriter, r *http.Request) {
//		event, err := verifier.VerifyAndDecode(r)
//		if err != nil {
//			http.Error(w, "invalid webhook", http.StatusUnauthorized)
//			return
//		}
//		switch {
//		case event.InboundEmail != nil:
//			// DELIVER_WEBHOOK: a full inbound message.
//		case event.DeliveryStatus != nil:
//			// NOTIFY_WEBHOOK: an SMTP delivery status.
//		case event.StorageStatus != nil:
//			// NOTIFY_WEBHOOK: an object-storage delivery status.
//		}
//	}
//
// The signing secret is shown only once, when the webhook endpoint is created
// or its secret is rotated. Store it and pass it to [WithSecret]. The secret
// is used as literal key bytes; do not base64-decode it.
package webhook
