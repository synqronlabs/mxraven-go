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

func ExampleClient_Workspace() {
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

	ws := client.Workspace("acme")
	tenant, err := ws.Tenant.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(ws.Slug(), tenant.Slug, tenant.Status)

	// Output: acme acme active
}

func ExampleDomainsService_List() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + testDomainJSON + "]"))
	}))
	defer srv.Close()

	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithToken("secret"),
	)
	if err != nil {
		log.Fatal(err)
	}

	domains, err := admin.Take(client.Workspace("acme").Domains.List(
		context.Background(),
		admin.ListDomainsOptions{},
	), 10)
	if err != nil {
		log.Fatal(err)
	}
	for _, domain := range domains {
		fmt.Println(domain.DomainName)
	}

	// Output: example.com
}

func ExampleDomainsService_Pages() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + testDomainJSON + "]"))
	}))
	defer srv.Close()

	client, err := admin.New(
		admin.WithBaseURL(srv.URL),
		admin.WithToken("secret"),
	)
	if err != nil {
		log.Fatal(err)
	}

	pager := client.Workspace("acme").Domains.Pages(admin.ListDomainsOptions{})
	for pager.Next(context.Background()) {
		for _, domain := range pager.Items() {
			fmt.Println(domain.DomainName)
		}
	}
	if err := pager.Err(); err != nil {
		log.Fatal(err)
	}

	// Output: example.com
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
