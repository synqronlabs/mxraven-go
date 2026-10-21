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
	// PreviousToken is the opaque token that requests the preceding page. An
	// empty value means the page has no preceding page.
	PreviousToken string
	// LastToken is the opaque token that requests the last page, when the
	// control plane reports one.
	LastToken string
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
//	pager := ws.Domains.Pages(admin.ListDomainsOptions{})
//	for pager.Next(ctx) {
//		for _, domain := range pager.Items() {
//			fmt.Println(domain.DomainName)
//		}
//	}
//	if err := pager.Err(); err != nil {
//		return err
//	}
type Pager[T any] struct {
	fetch PageFetcher[T]
	page  Page[T]
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
	return newPager(fetch), nil
}

// newPager returns a Pager without validating fetch.
func newPager[T any](fetch PageFetcher[T]) *Pager[T] {
	return &Pager[T]{fetch: fetch}
}

// errorPager returns a Pager that yields err on the first call to Next.
func errorPager[T any](err error) *Pager[T] {
	return newPager(func(context.Context, string) (Page[T], error) {
		return Page[T]{}, err
	})
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
	p.page = page
	p.items = page.Items
	p.token = page.NextToken
	if p.token == "" {
		p.done = true
	}
	return true
}

// Page returns the most recently loaded page, including its cursor metadata.
func (p *Pager[T]) Page() Page[T] {
	return p.page
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

// PreviousToken returns the previous-page token reported by the most recently
// loaded page.
func (p *Pager[T]) PreviousToken() string {
	return p.page.PreviousToken
}

// LastToken returns the last-page token reported by the most recently loaded
// page.
func (p *Pager[T]) LastToken() string {
	return p.page.LastToken
}

// Err returns the error that stopped iteration, if any.
func (p *Pager[T]) Err() error {
	return p.err
}
