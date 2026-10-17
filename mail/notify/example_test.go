package notify_test

import (
	"context"
	"log"

	"github.com/synqronlabs/mxraven-go/mail/notify"
)

func ExampleClient_Consume() {
	client, err := notify.New(
		notify.WithURL("wss://nats.mxraven.com"),
		notify.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
		notify.WithListener("0b0b0b0b-1111-2222-3333-444444444444"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	err = client.Consume(context.Background(), func(_ context.Context, event notify.Event) error {
		switch {
		case event.Accepted != nil:
			log.Printf("accepted %s", event.Accepted.TaskID)
		case event.Attempt != nil:
			log.Printf("attempt %s: %s", event.Attempt.TaskID, event.Attempt.Status)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_Events() {
	client, err := notify.New(
		notify.WithURL("wss://nats.mxraven.com"),
		notify.WithCredentials("mxr_tx_ab12cd34ef56", "your-api-key-secret"),
		notify.WithListener("0b0b0b0b-1111-2222-3333-444444444444"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	iterator, err := client.Events(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = iterator.Close() }()

	for iterator.Next() {
		event := iterator.Event()
		if event.Accepted != nil {
			if err := iterator.Ack(); err != nil {
				log.Printf("acknowledge: %v", err)
			}
			continue
		}
		// A failure can be retried by the server.
		if err := iterator.Nak(); err != nil {
			log.Printf("retry: %v", err)
		}
	}
	if err := iterator.Err(); err != nil {
		log.Fatal(err)
	}
}
