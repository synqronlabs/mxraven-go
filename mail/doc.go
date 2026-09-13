// Package mail submits email through the mxRaven SMTP submission service and
// provides runtime helpers for the mail-facing mxRaven integration surfaces.
//
// The package wraps the open-source github.com/synqronlabs/raven SMTP client
// and message builder behind a small, mxRaven-specific API. A minimal
// submission looks like:
//
//	client, err := mail.New(
//		mail.WithAddress("smtp.mxraven.com", 587),
//		mail.WithCredentials(username, secret),
//	)
//	if err != nil {
//		return err
//	}
//	defer client.Close()
//
//	result, err := client.Send(ctx, mail.NewMessage().
//		From("Acme <noreply@acme.example>").
//		To("customer@example.com").
//		Subject("Your receipt").
//		Text("Thanks for your order."))
//	if err != nil {
//		return err
//	}
//	log.Printf("accepted as %s", result.MessageRef)
//
// Authentication uses an mxRaven submission API key. The username is the key's
// username, for example "mxr_tx_ab12cd34ef56", and the secret is the key's
// secret. The submission service requires STARTTLS and SMTP AUTH; both are
// enabled by default.
//
// Subpackages cover the remaining runtime surfaces:
//
//   - github.com/synqronlabs/mxraven-go/mail/webhook verifies and decodes
//     DELIVER_WEBHOOK and NOTIFY_WEBHOOK deliveries.
//   - github.com/synqronlabs/mxraven-go/mail/feedback submits recipient
//     feedback to the mxRaven feedback service.
//
// Control-plane administration, including suppression lists, deliverability
// reporting, and mail analytics, lives in
// github.com/synqronlabs/mxraven-go/admin.
package mail
