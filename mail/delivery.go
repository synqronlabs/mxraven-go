package mail

import (
	"errors"
	"fmt"
	"time"

	ravenmail "github.com/synqronlabs/raven/mail"
)

// DeliveryMode selects how the server handles a message that misses a
// DELIVERBY deadline (RFC 2852).
type DeliveryMode int

const (
	// DeliveryNotify requests a delayed-delivery notification if the deadline
	// passes. It is the zero value.
	DeliveryNotify DeliveryMode = iota
	// DeliveryReturn requests that the message be returned to the sender if the
	// deadline passes.
	DeliveryReturn
)

// DeliveryBy requests delivery within a bounded time window using the DELIVERBY
// SMTP extension (RFC 2852). The submission server must advertise DELIVERBY or
// the send fails with [ErrDeliveryByUnsupported].
type DeliveryBy struct {
	// Within is the deadline relative to the time the server receives the
	// message. It is truncated to whole seconds; sub-second precision is
	// discarded.
	Within time.Duration
	// Mode selects how a missed deadline is handled.
	Mode DeliveryMode
	// Trace requests that the delivery-by time be recorded in trace
	// information.
	Trace bool
}

// raven converts the deadline into a raven envelope parameter, applying the
// RFC 2852 rules the server enforces.
func (d DeliveryBy) raven() (*ravenmail.DeliveryBy, error) {
	if d.Within < 0 {
		return nil, errors.New("mail: delivery-by deadline must not be negative")
	}

	seconds := int64(d.Within / time.Second)
	mode := ravenmail.DeliveryByModeNotify
	switch d.Mode {
	case DeliveryNotify:
	case DeliveryReturn:
		if seconds <= 0 {
			return nil, errors.New("mail: delivery-by return mode requires a deadline of at least one second")
		}
		mode = ravenmail.DeliveryByModeReturn
	default:
		return nil, fmt.Errorf("mail: invalid delivery-by mode %d", d.Mode)
	}

	return &ravenmail.DeliveryBy{
		Seconds: seconds,
		Mode:    mode,
		Trace:   d.Trace,
	}, nil
}
