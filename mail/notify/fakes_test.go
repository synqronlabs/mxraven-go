package notify

import (
	"context"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeMsg is a test double for jetstream.Msg.
type fakeMsg struct {
	subject   string
	data      []byte
	delivered uint64

	ackErr     error
	termErr    error
	nakErr     error
	acked      bool
	naked      bool
	nakDelay   time.Duration
	termed     bool
	inProgress bool
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.delivered}, nil
}

func (m *fakeMsg) Data() []byte         { return m.data }
func (m *fakeMsg) Headers() nats.Header { return nil }
func (m *fakeMsg) Subject() string      { return m.subject }
func (m *fakeMsg) Reply() string        { return "" }

func (m *fakeMsg) Ack() error {
	m.acked = true
	return m.ackErr
}

func (m *fakeMsg) DoubleAck(context.Context) error { return m.Ack() }

func (m *fakeMsg) Nak() error {
	m.naked = true
	return m.nakErr
}

func (m *fakeMsg) NakWithDelay(delay time.Duration) error {
	m.naked = true
	m.nakDelay = delay
	return m.nakErr
}

func (m *fakeMsg) InProgress() error {
	m.inProgress = true
	return nil
}

func (m *fakeMsg) Term() error {
	m.termed = true
	return m.termErr
}

func (m *fakeMsg) TermWithReason(string) error { return m.Term() }

// fakeConsumeContext is a test double for jetstream.ConsumeContext.
type fakeConsumeContext struct {
	closed chan struct{}
	once   sync.Once
	onStop func()
}

func newFakeConsumeContext() *fakeConsumeContext {
	return &fakeConsumeContext{closed: make(chan struct{})}
}

func (c *fakeConsumeContext) Stop() {
	c.once.Do(func() {
		close(c.closed)
		if c.onStop != nil {
			c.onStop()
		}
	})
}

func (c *fakeConsumeContext) Drain()                  { c.Stop() }
func (c *fakeConsumeContext) Closed() <-chan struct{} { return c.closed }

// fakeMessages is a test double for jetstream.MessagesContext.
type fakeMessages struct {
	messages []jetstream.Msg
	err      error

	index   int
	stopped bool
}

func (m *fakeMessages) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	if m.stopped {
		return nil, jetstream.ErrMsgIteratorClosed
	}
	if m.index < len(m.messages) {
		msg := m.messages[m.index]
		m.index++
		return msg, nil
	}
	if m.err != nil {
		err := m.err
		m.err = nil
		return nil, err
	}
	return nil, jetstream.ErrMsgIteratorClosed
}

func (m *fakeMessages) Stop()  { m.stopped = true }
func (m *fakeMessages) Drain() { m.stopped = true }

// fakeSource is a test double for eventSource.
type fakeSource struct {
	consume  func(handler jetstream.MessageHandler) (jetstream.ConsumeContext, error)
	messages func() (jetstream.MessagesContext, error)
}

func (s *fakeSource) Consume(handler jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	return s.consume(handler)
}

func (s *fakeSource) Messages(_ ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return s.messages()
}

// testClient builds a Client wired to source without a network connection.
func testClient(source eventSource, errorHandler func(error)) *Client {
	return &Client{
		source:       func(context.Context) (eventSource, error) { return source, nil },
		consumerName: "notify-listener-1",
		errorHandler: errorHandler,
		done:         make(chan struct{}),
	}
}
