package mime_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/synqronlabs/mxraven-go/mail/mime"
	"github.com/synqronlabs/mxraven-go/mail/webhook"
)

func ExampleParseEmail() {
	message := "Subject: Receipt\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Thanks for your order."
	email, err := mime.ParseEmail([]byte(message))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(email.Subject)
	fmt.Println(email.Text)

	// Output:
	// Receipt
	// Thanks for your order.
}

func ExampleFetchAndParseRawEmail() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Subject: Inbound\r\nContent-Type: text/plain\r\n\r\nhello"))
	}))
	defer server.Close()

	email, err := mime.FetchAndParseRawEmail(context.Background(), webhook.RawEmail{
		URL:         server.URL,
		AccessToken: "token",
	}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(email.Subject)

	// Output: Inbound
}
