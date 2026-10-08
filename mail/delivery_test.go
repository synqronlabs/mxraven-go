package mail

import (
	"errors"
	"testing"
	"time"

	ravenclient "github.com/synqronlabs/raven/client"
	ravenmail "github.com/synqronlabs/raven/mail"
)

func TestDeliveryByRaven(t *testing.T) {
	tests := []struct {
		name    string
		in      DeliveryBy
		wantErr bool
		seconds int64
		mode    ravenmail.DeliveryByMode
		trace   bool
	}{
		{
			name:    "notify",
			in:      DeliveryBy{Within: 90 * time.Second, Mode: DeliveryNotify},
			seconds: 90,
			mode:    ravenmail.DeliveryByModeNotify,
		},
		{
			name:    "return with trace",
			in:      DeliveryBy{Within: 2 * time.Hour, Mode: DeliveryReturn, Trace: true},
			seconds: 7200,
			mode:    ravenmail.DeliveryByModeReturn,
			trace:   true,
		},
		{
			name:    "truncates sub-second",
			in:      DeliveryBy{Within: 1500 * time.Millisecond, Mode: DeliveryNotify},
			seconds: 1,
			mode:    ravenmail.DeliveryByModeNotify,
		},
		{
			name:    "zero notify deadline",
			in:      DeliveryBy{Within: 0, Mode: DeliveryNotify},
			seconds: 0,
			mode:    ravenmail.DeliveryByModeNotify,
		},
		{
			name:    "zero return deadline",
			in:      DeliveryBy{Within: 0, Mode: DeliveryReturn},
			wantErr: true,
		},
		{
			name:    "sub-second return deadline",
			in:      DeliveryBy{Within: 500 * time.Millisecond, Mode: DeliveryReturn},
			wantErr: true,
		},
		{
			name:    "negative deadline",
			in:      DeliveryBy{Within: -time.Second, Mode: DeliveryNotify},
			wantErr: true,
		},
		{
			name:    "invalid mode",
			in:      DeliveryBy{Within: time.Minute, Mode: DeliveryMode(42)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.raven()
			if tt.wantErr {
				if err == nil {
					t.Fatal("raven() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("raven() error = %v", err)
			}
			if got.Seconds != tt.seconds || got.Mode != tt.mode || got.Trace != tt.trace {
				t.Errorf("raven() = %+v, want seconds=%d mode=%s trace=%v",
					got, tt.seconds, tt.mode, tt.trace)
			}
		})
	}
}

func TestEnvelopeRavenDeliveryBy(t *testing.T) {
	envelope, err := Envelope{
		From:       "sender@example.com",
		To:         []string{"recipient@example.com"},
		DeliveryBy: &DeliveryBy{Within: time.Hour, Mode: DeliveryReturn},
	}.raven()
	if err != nil {
		t.Fatalf("raven() error = %v", err)
	}
	if envelope.DeliveryBy == nil {
		t.Fatal("Envelope.DeliveryBy = nil")
	}
	if envelope.DeliveryBy.Seconds != 3600 || envelope.DeliveryBy.Mode != ravenmail.DeliveryByModeReturn {
		t.Errorf("Envelope.DeliveryBy = %+v", envelope.DeliveryBy)
	}

	_, err = Envelope{
		From:       "sender@example.com",
		To:         []string{"recipient@example.com"},
		DeliveryBy: &DeliveryBy{Within: 0, Mode: DeliveryReturn},
	}.raven()
	if err == nil {
		t.Fatal("raven() with invalid delivery-by error = nil, want error")
	}
}

func TestMessageBuildDeliveryBy(t *testing.T) {
	built, err := NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Text("time-sensitive").
		DeliveryBy(DeliveryBy{Within: 30 * time.Minute, Mode: DeliveryReturn, Trace: true}).
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	got := built.Envelope.DeliveryBy
	if got == nil {
		t.Fatal("Envelope.DeliveryBy = nil")
	}
	if got.Seconds != 1800 || got.Mode != ravenmail.DeliveryByModeReturn || !got.Trace {
		t.Errorf("Envelope.DeliveryBy = %+v", got)
	}
}

func TestMessageBuildDeliveryByInvalid(t *testing.T) {
	_, err := NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Text("time-sensitive").
		DeliveryBy(DeliveryBy{Within: 0, Mode: DeliveryReturn}).
		build()
	if err == nil {
		t.Fatal("build() error = nil, want error")
	}
}

func TestTranslateDeliveryUnsupported(t *testing.T) {
	err := translateSMTPError(ravenclient.ErrDeliveryByNotSupported)
	if !errors.Is(err, ErrDeliveryByUnsupported) {
		t.Fatalf("translateSMTPError() = %v, want ErrDeliveryByUnsupported", err)
	}
}
