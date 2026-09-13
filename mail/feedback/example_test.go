package feedback_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/synqronlabs/mxraven-go/mail/feedback"
)

func ExampleClient_LearnSpam() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"learned","disposition":"spam"}`)
	}))
	defer server.Close()

	client, err := feedback.New(
		feedback.WithBaseURL(server.URL),
		feedback.WithCredentials("mxr_tx_ab12cd34ef56", "secret"),
	)
	if err != nil {
		fmt.Println("client:", err)
		return
	}

	result, err := client.LearnSpam(context.Background(), strings.NewReader("raw RFC 822 message"))
	if err != nil {
		fmt.Println("learn:", err)
		return
	}
	fmt.Println(result.Status, result.Disposition)

	// Output: learned spam
}
