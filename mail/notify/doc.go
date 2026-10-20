// Package notify consumes mxRaven client notification events over NATS.
//
// A listener whose notifications are enabled publishes mail events to a
// JetStream stream. Each event is durable and acknowledged as it is consumed,
// and an event that is not acknowledged is redelivered until its 24-hour
// retention window expires. [Client] reads one listener's events from the
// per-listener durable consumer that the control plane creates when the setting
// is enabled.
//
// The connection authenticates with the listener's existing API key: the key
// prefix is the NATS user name and the key secret is the password. The server
// authorizes the connection through its auth callout and grants it only its own
// consumer's pull, info, and acknowledgement subjects and a per-listener
// inbox prefix.
//
// A minimal consumer looks like:
//
//	client, err := notify.New(
//		notify.WithURL("wss://notifications.mxraven.com"),
//		notify.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
//		notify.WithListener(listenerID),
//	)
//	if err != nil {
//		return err
//	}
//	defer client.Close()
//
//	return client.Consume(ctx, func(ctx context.Context, event notify.Event) error {
//		switch {
//		case event.Accepted != nil:
//			log.Printf("accepted %s", event.Accepted.TaskID)
//		case event.Attempt != nil:
//			log.Printf("attempt %s: %s", event.Attempt.TaskID, event.Attempt.Status)
//		}
//		return nil
//	})
//
// [Client.Consume] acknowledges an event when its handler returns nil, retries
// it with backoff when the handler returns an error, and stops cleanly when the
// handler returns [ErrStop]. [Client.Events] is a lower-level iterator for
// callers that want to acknowledge each event themselves.
//
// Accepted events reuse the webhook payload model
// ([github.com/synqronlabs/mxraven-go/mail/webhook.InboundEmail]); an MTA
// listener's accepted event carries raw-message access that
// [github.com/synqronlabs/mxraven-go/mail/webhook.RawEmail.Fetch] downloads.
// Attempt events are described by [AttemptEvent].
//
// The consumer and its subject are managed by the control plane. Read them,
// including the durable consumer name, with
// [github.com/synqronlabs/mxraven-go/admin.Client.GetListenerNotifications]
// and enable them with
// [github.com/synqronlabs/mxraven-go/admin.Client.UpdateListenerNotifications].
package notify
