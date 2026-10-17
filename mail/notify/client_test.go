package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestConsumeDispatchesAndAcknowledges(t *testing.T) {
	msg := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte(acceptedBody), delivered: 1}
	source := &fakeSource{consume: func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		consumeCtx := newFakeConsumeContext()
		handler(msg)
		consumeCtx.Stop()
		return consumeCtx, nil
	}}
	client := testClient(source, nil)

	var got Event
	if err := client.Consume(context.Background(), func(_ context.Context, event Event) error {
		got = event
		return nil
	}); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if got.Kind != KindAccepted || got.Accepted == nil {
		t.Fatalf("event = %+v, want accepted", got)
	}
	if !msg.acked {
		t.Error("message was not acknowledged")
	}
}

func TestConsumeHandlerErrorRetriesWithBackoff(t *testing.T) {
	tests := []struct {
		delivered uint64
		want      time.Duration
	}{
		{delivered: 0, want: defaultRetryBase},
		{delivered: 1, want: defaultRetryBase},
		{delivered: 2, want: 2 * time.Second},
		{delivered: 3, want: 4 * time.Second},
		{delivered: 100, want: defaultRetryMax},
	}
	for _, tt := range tests {
		msg := &fakeMsg{subject: "tenant.t.listener.l.events.attempt", data: []byte(smtpAttemptBody), delivered: tt.delivered}
		source := &fakeSource{consume: func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
			consumeCtx := newFakeConsumeContext()
			handler(msg)
			consumeCtx.Stop()
			return consumeCtx, nil
		}}
		client := testClient(source, nil)
		if err := client.Consume(context.Background(), func(context.Context, Event) error {
			return errors.New("transient")
		}); err != nil {
			t.Fatalf("Consume() error = %v", err)
		}
		if !msg.naked {
			t.Fatalf("delivered %d: message was not negatively acknowledged", tt.delivered)
		}
		if msg.nakDelay != tt.want {
			t.Errorf("delivered %d: nak delay = %v, want %v", tt.delivered, msg.nakDelay, tt.want)
		}
	}
}

func TestConsumeErrStopHalts(t *testing.T) {
	msg := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte(acceptedBody), delivered: 1}
	consumeCtx := newFakeConsumeContext()
	source := &fakeSource{consume: func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		handler(msg)
		return consumeCtx, nil
	}}
	client := testClient(source, nil)

	if err := client.Consume(context.Background(), func(context.Context, Event) error {
		return ErrStop
	}); err != nil {
		t.Fatalf("Consume() error = %v, want nil", err)
	}
	if !msg.acked {
		t.Error("stop event was not acknowledged")
	}
	if msg.naked {
		t.Error("stop event was negatively acknowledged")
	}
}

func TestConsumeStopsOnUndecodableEvent(t *testing.T) {
	msg := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte("{"), delivered: 1}
	consumeCtx := newFakeConsumeContext()
	source := &fakeSource{consume: func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		handler(msg)
		return consumeCtx, nil
	}}
	client := testClient(source, nil)

	if err := client.Consume(context.Background(), func(context.Context, Event) error {
		return nil
	}); err != nil {
		t.Fatalf("Consume() error = %v, want nil", err)
	}
	if !msg.termed {
		t.Error("undecodable event was not terminated")
	}
}

func TestConsumeReportsUndecodableEventWithHandler(t *testing.T) {
	msg := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte("{"), delivered: 1}
	consumeCtx := newFakeConsumeContext()
	source := &fakeSource{consume: func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		handler(msg)
		consumeCtx.Stop()
		return consumeCtx, nil
	}}
	var reported error
	client := testClient(source, func(err error) { reported = err })

	if err := client.Consume(context.Background(), func(context.Context, Event) error {
		return nil
	}); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	var decodeErr *DecodeError
	if !errors.As(reported, &decodeErr) {
		t.Fatalf("reported error = %v, want *DecodeError", reported)
	}
	if !msg.termed {
		t.Error("undecodable event was not terminated")
	}
}

func TestConsumeHonoursContextCancellation(t *testing.T) {
	consumeCtx := newFakeConsumeContext()
	source := &fakeSource{consume: func(jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		return consumeCtx, nil
	}}
	client := testClient(source, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Consume(ctx, func(context.Context, Event) error { return nil })
	}()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Consume() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Consume() did not return after cancellation")
	}
}

func TestConsumeReturnsWhenClientCloses(t *testing.T) {
	consumeCtx := newFakeConsumeContext()
	source := &fakeSource{consume: func(jetstream.MessageHandler) (jetstream.ConsumeContext, error) {
		return consumeCtx, nil
	}}
	client := testClient(source, nil)

	done := make(chan error, 1)
	go func() {
		done <- client.Consume(context.Background(), func(context.Context, Event) error { return nil })
	}()
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Consume() error = %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Consume() did not return after Close")
	}
}

func TestConsumeRejectsNilHandler(t *testing.T) {
	client := testClient(&fakeSource{}, nil)
	if err := client.Consume(context.Background(), nil); err == nil {
		t.Fatal("Consume() error = nil, want error")
	}
}

func TestConsumePropagatesResolveError(t *testing.T) {
	want := errors.New("boom")
	client := &Client{
		source: func(context.Context) (eventSource, error) { return nil, want },
		done:   make(chan struct{}),
	}
	if err := client.Consume(context.Background(), func(context.Context, Event) error { return nil }); !errors.Is(err, want) {
		t.Fatalf("Consume() error = %v, want %v", err, want)
	}
}
