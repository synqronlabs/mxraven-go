package webhook_test

import (
	"fmt"

	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

func ExampleDecode() {
	body := []byte(`{
		"event_type": "inbound_email",
		"task_id": "task-123",
		"routing_decision": {
			"terminal_action": "TERMINAL_ACTION_TYPE_RELAY",
			"used_listener_default": false
		}
	}`)

	event, err := webhook.Decode(body)
	if err != nil {
		fmt.Println("decode failed")
		return
	}
	fmt.Println(event.Type)
	fmt.Println(event.InboundEmail.RoutingDecision.TerminalAction)

	// Output:
	// inbound_email
	// TERMINAL_ACTION_TYPE_RELAY
}
