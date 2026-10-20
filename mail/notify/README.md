# mxRaven Client Notifications SDK

A Go client for the mxRaven client notification stream. It consumes the durable
mail events a listener publishes over NATS JetStream, using the same listener
API key as SMTP submission.

## Install

```sh
go get github.com/synqronlabs/mxraven-go/mail/notify
```

Requires Go 1.26.4 or later (see `go.mod`).

## Before you connect

Notifications are off by default. Enable them for a listener with the `admin`
SDK and read back the coordinates the client needs:

```go
res, err := adminClient.UpdateListenerNotifications(ctx, &api.UpdateListenerNotificationsRequest{
	Enabled: true,
}, api.UpdateListenerNotificationsParams{Slug: "acme", ListenerID: listenerID})
```

Enabling creates the listener's durable consumer. The read returns:

| Field | Meaning |
| --- | --- |
| `enabled` | Whether the listener publishes events. |
| `subject` | The subject prefix the listener's events are published on. |
| `consumer` | The durable pull consumer a client binds to. |

The consumer name is also derivable as `notify-<listener_id>`, which is what
`notify.WithListener` uses by default.

## Connecting

```go
client, err := notify.New(
	notify.WithURL("wss://notifications.mxraven.com"),
	notify.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
	notify.WithListener(listenerID),
)
if err != nil {
	return err
}
defer client.Close()
```

The URL is the client-facing NATS WebSocket endpoint. Authentication reuses the
listener's API key: the key prefix is the NATS user name and the key secret is
the password. The server authorizes the connection through its auth callout and
grants it only its own consumer's pull, info, and acknowledgement subjects plus
a per-listener inbox prefix.

`New` connects immediately, so an invalid or revoked key fails here rather than
on the first event. After a successful connection the client reconnects
indefinitely if the tier is unreachable, and the consumer resumes from its
acknowledgement floor. Use `notify.WithConsumer` to bind to the exact consumer
name the control plane reported instead of the derived default.

## Consuming events

`Consume` dispatches each decoded event to a handler and manages
acknowledgement:

- a handler that returns `nil` acknowledges the event;
- a handler that returns another error leaves the event unacknowledged, and the
  server redelivers it with exponential backoff;
- a handler that returns `notify.ErrStop` acknowledges the event and stops
  consumption;
- an event the SDK cannot decode is terminated so it cannot poison the stream,
  and `Consume` stops with a `*notify.DecodeError` unless an error handler has
  been set.

```go
return client.Consume(ctx, func(ctx context.Context, event notify.Event) error {
	switch {
	case event.Accepted != nil:
		return handleAccepted(ctx, event.Accepted)
	case event.Attempt != nil:
		return handleAttempt(ctx, event.Attempt)
	}
	return nil
})
```

`Consume` returns `ctx.Err()` when the context is cancelled, `notify.ErrClosed`
when the client is closed, and a wrapped terminal error if the consumer stops on
its own.

## Iterating events

`Events` is the lower-level form for callers that want to acknowledge each event
themselves:

```go
iterator, err := client.Events(ctx)
if err != nil {
	return err
}
defer iterator.Close()

for iterator.Next() {
	event := iterator.Event()
	if err := process(event); err != nil {
		_ = iterator.Nak()
		continue
	}
	if err := iterator.Ack(); err != nil {
		return err
	}
}
if err := iterator.Err(); err != nil {
	return err
}
```

`Nak` redelivers immediately; `NakWithDelay` redelivers after a delay; `Term`
tells the server never to redeliver the event. An event left unacknowledged is
redelivered after the consumer's acknowledgement timeout.

## Events

Both event kinds arrive on the listener's single durable consumer.

### Accepted events

An accepted event is the same payload mxRaven posts to a webhook endpoint, so it
is decoded into `*webhook.InboundEmail`: task and listener identifiers,
acceptance and occurrence timestamps, the routing decision, verdicts, the SMTP
envelope, the parsed message summary and headers, and raw-message access.

```go
if event.Accepted != nil {
	raw, err := event.Accepted.RawEmail.Fetch(ctx, nil)
	if err != nil {
		return err
	}
	_ = raw
}
```

Raw-message access is granted to MTA listeners only; a submission listener's
accepted event leaves the block empty. The embedded token is scoped to that one
message and outlives the stream's 24-hour retention window.

### Attempt events

An attempt event reports one delivery attempt for an accepted message. A rule
that chains side-effect actions reports each action it triggers, so one message
can produce several attempt events. `AttemptEvent.Channel` names the worker:

| `Channel` | Reported by |
| --- | --- |
| `notify.ChannelSMTP` | SMTP delivery |
| `notify.ChannelS3` | Object-storage write |
| `notify.ChannelWebhook` | Webhook delivery |

Every attempt carries `CorrelationID`, which is the `TaskID` of the accepted
event for the message the attempt stems from. A submission listener's attempts
already share the accepted event's `TaskID`.

## Retention and delivery

The stream keeps events for 24 hours from creation regardless of
acknowledgement. Acknowledging stops redelivery; it does not remove the event.
A client that is offline longer than the retention window cannot recover the
gap. Connections that share one API key share one consumer, so events are
distributed between them rather than duplicated to each.

## Options

| Option | Description |
| --- | --- |
| `WithURL(url string)` | Required. Client-facing NATS WebSocket URL (`ws://` or `wss://`). |
| `WithCredentials(username, secret string)` | Required. Listener API key. |
| `WithListener(listenerID string)` | Required. Listener whose events are consumed. |
| `WithConsumer(name string)` | Override the durable consumer name. |
| `WithTLSConfig(*tls.Config)` | WebSocket TLS configuration, for example to trust a private CA. |
| `WithConnectTimeout(timeout time.Duration)` | Connect timeout. Default 10s. |
| `WithErrorHandler(func(error))` | Non-fatal connection and decode errors. |

## Development

```sh
go test ./mail/notify/...
golangci-lint run ./mail/notify/...
```
