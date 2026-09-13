package admin

import (
	"context"
	"errors"
)

// Page is one page of a paginated list operation.
type Page[T any] struct {
	// Items holds the resources returned for the page.
	Items []T
	// NextToken is the opaque token that requests the following page. An empty
	// value means the page is the last one.
	NextToken string
}

// PageFetcher loads one page of a paginated list operation. An empty token
// requests the first page.
type PageFetcher[T any] func(ctx context.Context, token string) (Page[T], error)

// Pager iterates a paginated list operation one page at a time.
//
// The pager never fetches a page before it is requested, so callers can stop
// early without draining the collection or provoking rate-limit errors. A
// Pager is not safe for concurrent use.
//
//	fetch := func(ctx context.Context, token string) (admin.Page[api.Tenant], error) {
//		params := api.ListTenantsParams{}
//		if token != "" {
//			params.PageToken = api.OptString{Value: token, Set: true}
//		}
//		res, err := client.ListTenants(ctx, params)
//		if err != nil {
//			return admin.Page[api.Tenant]{}, err
//		}
//		if problem, ok := admin.ProblemFrom(res); ok {
//			return admin.Page[api.Tenant]{}, fmt.Errorf("list tenants: %s", problem.Code)
//		}
//		page := res.(*api.ListTenantsOKHeaders)
//		next, _ := admin.NextPageToken(res)
//		return admin.Page[api.Tenant]{Items: page.Response, NextToken: next}, nil
//	}
//	pager, err := admin.NewPager(fetch)
//	if err != nil {
//		return err
//	}
//	for pager.Next(ctx) {
//		for _, tenant := range pager.Items() {
//			// ...
//		}
//	}
//	if err := pager.Err(); err != nil {
//		return err
//	}
type Pager[T any] struct {
	fetch PageFetcher[T]
	token string
	items []T
	done  bool
	err   error
}

// NewPager returns a Pager that loads pages with fetch. fetch must not be nil.
func NewPager[T any](fetch PageFetcher[T]) (*Pager[T], error) {
	if fetch == nil {
		return nil, errors.New("admin: pager fetch function must not be nil")
	}
	return &Pager[T]{fetch: fetch}, nil
}

// Next loads the next page. It reports false when there are no more pages or
// when a page failed to load; use Err to distinguish the two.
func (p *Pager[T]) Next(ctx context.Context) bool {
	if p.done {
		return false
	}
	page, err := p.fetch(ctx, p.token)
	if err != nil {
		p.err = err
		p.done = true
		return false
	}
	p.items = page.Items
	p.token = page.NextToken
	if p.token == "" {
		p.done = true
	}
	return true
}

// Items returns the items of the most recently loaded page. It returns nil
// before the first successful call to Next.
func (p *Pager[T]) Items() []T {
	return p.items
}

// Token returns the token that will be sent to load the next page. It is empty
// before the first call to Next and after the last page has been loaded.
func (p *Pager[T]) Token() string {
	return p.token
}

// Err returns the error that stopped iteration, if any.
func (p *Pager[T]) Err() error {
	return p.err
}
