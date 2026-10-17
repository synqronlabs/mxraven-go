package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestIteratorYieldsEvents(t *testing.T) {
	accepted := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte(acceptedBody), delivered: 1}
	attempt := &fakeMsg{subject: "tenant.t.listener.l.events.attempt", data: []byte(smtpAttemptBody), delivered: 1}
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{messages: []jetstream.Msg{accepted, attempt}}, nil
	}}
	client := testClient(source, nil)

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = it.Close() }()

	if !it.Next() {
		t.Fatalf("Next() = false, want true; err = %v", it.Err())
	}
	if got := it.Event().Kind; got != KindAccepted {
		t.Fatalf("Kind = %q, want %q", got, KindAccepted)
	}
	if err := it.Ack(); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	if !it.Next() {
		t.Fatalf("Next() = false, want true; err = %v", it.Err())
	}
	if got := it.Event().Attempt.Channel; got != ChannelSMTP {
		t.Fatalf("Channel = %q, want %q", got, ChannelSMTP)
	}
	if err := it.NakWithDelay(0); err != nil {
		t.Fatalf("NakWithDelay() error = %v", err)
	}

	if it.Next() {
		t.Fatal("Next() = true, want false")
	}
	if err := it.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if !accepted.acked {
		t.Error("accepted event was not acknowledged")
	}
	if !attempt.naked {
		t.Error("attempt event was not negatively acknowledged")
	}
}

func TestIteratorStopsOnUndecodableEvent(t *testing.T) {
	bad := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte("{"), delivered: 1}
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{messages: []jetstream.Msg{bad}}, nil
	}}
	client := testClient(source, nil)

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = it.Close() }()

	if it.Next() {
		t.Fatal("Next() = true, want false")
	}
	var decodeErr *DecodeError
	if !errors.As(it.Err(), &decodeErr) {
		t.Fatalf("Err() = %v, want *DecodeError", it.Err())
	}
	if !bad.termed {
		t.Error("undecodable event was not terminated")
	}
}

func TestIteratorSkipsUndecodableEventWithHandler(t *testing.T) {
	bad := &fakeMsg{subject: "tenant.t.listener.l.events.accepted", data: []byte("{"), delivered: 1}
	good := &fakeMsg{subject: "tenant.t.listener.l.events.attempt", data: []byte(s3AttemptBody), delivered: 1}
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{messages: []jetstream.Msg{bad, good}}, nil
	}}
	var reported error
	client := testClient(source, func(err error) { reported = err })

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = it.Close() }()

	if !it.Next() {
		t.Fatalf("Next() = false, want true; err = %v", it.Err())
	}
	if got := it.Event().Attempt.Channel; got != ChannelS3 {
		t.Fatalf("Channel = %q, want %q", got, ChannelS3)
	}
	var decodeErr *DecodeError
	if !errors.As(reported, &decodeErr) {
		t.Fatalf("reported = %v, want *DecodeError", reported)
	}
	if !bad.termed {
		t.Error("undecodable event was not terminated")
	}
}

func TestIteratorSurfacesTerminalError(t *testing.T) {
	want := errors.New("boom")
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{err: want}, nil
	}}
	client := testClient(source, nil)

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = it.Close() }()

	if it.Next() {
		t.Fatal("Next() = true, want false")
	}
	if !errors.Is(it.Err(), want) {
		t.Fatalf("Err() = %v, want %v", it.Err(), want)
	}
}

func TestIteratorAckWithoutCurrentEvent(t *testing.T) {
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{}, nil
	}}
	client := testClient(source, nil)

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	defer func() { _ = it.Close() }()

	if err := it.Ack(); !errors.Is(err, errNoEvent) {
		t.Fatalf("Ack() error = %v, want errNoEvent", err)
	}
	if err := it.Nak(); !errors.Is(err, errNoEvent) {
		t.Fatalf("Nak() error = %v, want errNoEvent", err)
	}
	if err := it.Term(); !errors.Is(err, errNoEvent) {
		t.Fatalf("Term() error = %v, want errNoEvent", err)
	}
}

func TestIteratorCloseIsIdempotent(t *testing.T) {
	source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
		return &fakeMessages{}, nil
	}}
	client := testClient(source, nil)

	it, err := client.Events(context.Background())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("Close() second error = %v", err)
	}
}

func TestIteratorAcknowledgementErrors(t *testing.T) {
	tests := []struct {
		name string
		msg  *fakeMsg
		call func(*Iterator) error
	}{
		{name: "ack", msg: &fakeMsg{ackErr: errors.New("ack failed")}, call: func(it *Iterator) error { return it.Ack() }},
		{name: "nak", msg: &fakeMsg{nakErr: errors.New("nak failed")}, call: func(it *Iterator) error { return it.Nak() }},
		{name: "term", msg: &fakeMsg{termErr: errors.New("term failed")}, call: func(it *Iterator) error { return it.Term() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.msg.subject = "tenant.t.listener.l.events.accepted"
			tt.msg.data = []byte(acceptedBody)
			tt.msg.delivered = 1
			source := &fakeSource{messages: func() (jetstream.MessagesContext, error) {
				return &fakeMessages{messages: []jetstream.Msg{tt.msg}}, nil
			}}
			client := testClient(source, nil)

			it, err := client.Events(context.Background())
			if err != nil {
				t.Fatalf("Events() error = %v", err)
			}
			defer func() { _ = it.Close() }()

			if !it.Next() {
				t.Fatalf("Next() = false, want true; err = %v", it.Err())
			}
			if err := tt.call(it); err == nil {
				t.Fatal("call() error = nil, want error")
			}
		})
	}
}
