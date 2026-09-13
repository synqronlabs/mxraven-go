package admin_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/synqronlabs/mxraven-go/admin"
	"github.com/synqronlabs/mxraven-go/admin/api"
)

func ExampleNew() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testTenantJSON))
	}))
	defer srv.Close()

	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithToken("secret"),
	)
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		log.Fatal(err)
	}
	if tenant, ok := res.(*api.TenantHeaders); ok {
		fmt.Println(tenant.Response.Slug)
	}

	// Output: acme
}

func ExampleProblemFrom() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(testProblemJSON))
	}))
	defer srv.Close()

	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithToken("secret"),
	)
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.GetTenant(context.Background(), api.GetTenantParams{Slug: "acme"})
	if err != nil {
		log.Fatal(err)
	}
	if problem, ok := admin.ProblemFrom(res); ok {
		fmt.Println(problem.Code, problem.Status)
	}

	// Output: not_found 404
}
