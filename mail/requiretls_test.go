package mail

import "testing"

func TestMessageBuildRequireTLS(t *testing.T) {
	built, err := NewMessage().
		From("sender@example.com").
		To("recipient@example.com").
		Text("sensitive").
		RequireTLS().
		build()
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if !built.Envelope.RequireTLS {
		t.Error("Envelope.RequireTLS = false, want true")
	}
}

func TestEnvelopeRavenRequireTLS(t *testing.T) {
	envelope, err := Envelope{
		From:       "sender@example.com",
		To:         []string{"recipient@example.com"},
		RequireTLS: true,
	}.raven()
	if err != nil {
		t.Fatalf("raven() error = %v", err)
	}
	if !envelope.RequireTLS {
		t.Error("Envelope.RequireTLS = false, want true")
	}
}
