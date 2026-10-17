package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// errNoEvent reports an acknowledgement attempt without a current event.
var errNoEvent = errors.New("notify: no current event")

// eventSource is the slice of a JetStream consumer the SDK uses. It is
// satisfied by [github.com/nats-io/nats.go/jetstream.Consumer].
type eventSource interface {
	Consume(handler jetstream.MessageHandler, opts ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error)
	Messages(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error)
}

// Iterator reads events from a listener's durable consumer one at a time.
//
// Unlike [Client.Consume], an Iterator does not acknowledge events
// automatically: the caller decides whether each event is acknowledged with
// [Iterator.Ack], redelivered with [Iterator.Nak], or terminated with
// [Iterator.Term]. An event that is left unacknowledged is redelivered by the
// server after the consumer's acknowledgement timeout.
//
// An Iterator is not safe for concurrent use. Call [Iterator.Close] when
// finished.
type Iterator struct {
	ctx          context.Context
	client       *Client
	messages     jetstream.MessagesContext
	errorHandler func(error)

	current jetstream.Msg
	event   Event
	err     error
	closed  bool
	done    bool

	closeOnce sync.Once
}

// Events returns an Iterator over the listener's notification events. The
// iterator stops when ctx is cancelled, the client is closed, the consumer
// stops, or [Iterator.Close] is called.
//
// The caller is responsible for acknowledging each event.
func (c *Client) Events(ctx context.Context) (*Iterator, error) {
	if c == nil {
		return nil, ErrClosed
	}
	source, err := c.resolve(ctx)
	if err != nil {
		return nil, err
	}
	messages, err := source.Messages()
	if err != nil {
		return nil, fmt.Errorf("notify: start events: %w", err)
	}
	return &Iterator{
		ctx:          ctx,
		client:       c,
		messages:     messages,
		errorHandler: c.errorHandler,
	}, nil
}

// Next advances to the next event. It reports false when the iterator is
// stopped or fails; use [Iterator.Err] to distinguish the two.
func (it *Iterator) Next() bool {
	if it.done || it.closed {
		return false
	}
	for {
		msg, err := it.messages.Next(jetstream.NextContext(it.ctx))
		if err != nil {
			it.done = true
			it.err = it.translate(err)
			return false
		}
		event, err := decodeEvent(msg.Subject(), msg.Data())
		if err != nil {
			// Terminate a body the SDK cannot decode rather than redelivering it
			// forever. Stop when no handler has taken responsibility for it.
			_ = msg.Term()
			if it.errorHandler == nil {
				it.done = true
				it.err = err
				return false
			}
			it.errorHandler(err)
			continue
		}
		it.current = msg
		it.event = event
		return true
	}
}

// Event returns the event delivered by the most recent call to [Iterator.Next].
func (it *Iterator) Event() Event {
	return it.event
}

// Ack acknowledges the current event. A successfully acknowledged event is not
// redelivered.
func (it *Iterator) Ack() error {
	if it.current == nil {
		return errNoEvent
	}
	if err := it.current.Ack(); err != nil {
		return fmt.Errorf("notify: acknowledge event: %w", err)
	}
	it.current = nil
	return nil
}

// Nak negatively acknowledges the current event so the server redelivers it
// immediately.
func (it *Iterator) Nak() error {
	if it.current == nil {
		return errNoEvent
	}
	if err := it.current.Nak(); err != nil {
		return fmt.Errorf("notify: negate event: %w", err)
	}
	it.current = nil
	return nil
}

// NakWithDelay negatively acknowledges the current event so the server
// redelivers it after delay.
func (it *Iterator) NakWithDelay(delay time.Duration) error {
	if delay <= 0 {
		return it.Nak()
	}
	if it.current == nil {
		return errNoEvent
	}
	if err := it.current.NakWithDelay(delay); err != nil {
		return fmt.Errorf("notify: negate event with delay: %w", err)
	}
	it.current = nil
	return nil
}

// Term tells the server not to redeliver the current event. Use it for an event
// that will never succeed, such as one the caller cannot process.
func (it *Iterator) Term() error {
	if it.current == nil {
		return errNoEvent
	}
	if err := it.current.Term(); err != nil {
		return fmt.Errorf("notify: terminate event: %w", err)
	}
	it.current = nil
	return nil
}

// Err returns the error that stopped iteration, if any.
func (it *Iterator) Err() error {
	return it.err
}

// Close stops the iterator. Events that were not acknowledged are redelivered
// later. Close is safe to call more than once.
func (it *Iterator) Close() error {
	if it == nil {
		return nil
	}
	it.closeOnce.Do(func() {
		it.closed = true
		it.messages.Stop()
	})
	return nil
}

// translate maps a consumption error onto the SDK's error set.
func (it *Iterator) translate(err error) error {
	if it.client != nil && clientClosed(it.client) {
		return ErrClosed
	}
	if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
		return nil
	}
	return err
}

// clientClosed reports whether the client has been closed.
func clientClosed(c *Client) bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}
