package admin

import (
	"context"
	"iter"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// ListDomainsOptions filters and paginates [DomainsService.List].
//
// The zero value lists every domain with the server's default page size.
type ListDomainsOptions struct {
	ListOptions
	// Search is a case-insensitive search across the domain name. It must be at
	// least [MinSearchLength] characters when non-empty.
	Search string
	// Status restricts results to one lifecycle status.
	Status api.DomainStatus
	// SPFVerified restricts results by SPF verification state.
	SPFVerified *bool
	// DKIMVerified restricts results by DKIM verification state.
	DKIMVerified *bool
	// DMARCVerified restricts results by DMARC verification state.
	DMARCVerified *bool
	// SendingEnabled restricts results by outbound sending state.
	SendingEnabled *bool
}

// apply validates the options and copies them into params.
func (o ListDomainsOptions) apply(params *api.ListDomainsParams) error {
	if err := o.ListOptions.apply(&params.PageSize); err != nil {
		return err
	}
	search, err := normalizeSearch(o.Search)
	if err != nil {
		return err
	}
	if search != "" {
		params.Q = api.NewOptString(search)
	}
	if o.Status != "" {
		params.Status = api.NewOptDomainStatus(o.Status)
	}
	if o.SPFVerified != nil {
		params.SpfVerified = api.NewOptBool(*o.SPFVerified)
	}
	if o.DKIMVerified != nil {
		params.DkimVerified = api.NewOptBool(*o.DKIMVerified)
	}
	if o.DMARCVerified != nil {
		params.DmarcVerified = api.NewOptBool(*o.DMARCVerified)
	}
	if o.SendingEnabled != nil {
		params.SendingEnabled = api.NewOptBool(*o.SendingEnabled)
	}
	return nil
}

// CreateDomainInput describes a domain to onboard for outbound sending.
type CreateDomainInput struct {
	// DomainName is the domain to onboard.
	DomainName string
	// DMARCReportAddress optionally receives DMARC aggregate reports.
	DMARCReportAddress *string
}

// request converts the input to the generated request model.
func (in CreateDomainInput) request() api.CreateDomainRequest {
	request := api.CreateDomainRequest{DomainName: in.DomainName}
	if in.DMARCReportAddress != nil {
		request.DmarcReportAddress = api.NewOptString(*in.DMARCReportAddress)
	}
	return request
}

// DomainsService manages the sending domains of one tenant.
//
// Obtain one from [Workspace]. It is safe for concurrent use.
type DomainsService struct {
	client *Client
	slug   string
}

// List returns a lazy sequence of domains.
//
// Pages are fetched only as the sequence is consumed. A validation, transport,
// or API error is yielded once and stops iteration:
//
//	for domain, err := range ws.Domains.List(ctx, admin.ListDomainsOptions{}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(domain.DomainName)
//	}
func (s *DomainsService) List(ctx context.Context, opts ListDomainsOptions) iter.Seq2[api.Domain, error] {
	params := api.ListDomainsParams{Slug: api.TenantSlug(s.slug)}
	if err := opts.apply(&params); err != nil {
		return errorSeq[api.Domain](err)
	}
	return paginate(ctx, opts.PageToken, func(ctx context.Context, token string) ([]api.Domain, string, error) {
		page, err := s.fetchPage(ctx, params, token)
		if err != nil {
			return nil, "", err
		}
		return page.Items, page.NextToken, nil
	})
}

// Pages returns a page-level iterator over domains.
//
// The first page is fetched when Next is called. Use [Pager.Page] for the
// cursor metadata of the most recently loaded page.
func (s *DomainsService) Pages(opts ListDomainsOptions) *Pager[api.Domain] {
	params := api.ListDomainsParams{Slug: api.TenantSlug(s.slug)}
	if err := opts.apply(&params); err != nil {
		return errorPager[api.Domain](err)
	}
	return newPager(func(ctx context.Context, token string) (Page[api.Domain], error) {
		return s.fetchPage(ctx, params, token)
	})
}

func (s *DomainsService) fetchPage(ctx context.Context, params api.ListDomainsParams, token string) (Page[api.Domain], error) {
	pageParams := params
	if token != "" {
		pageParams.PageToken = api.NewOptString(token)
	}
	res, err := s.client.ListDomains(ctx, pageParams)
	if err != nil {
		return Page[api.Domain]{}, err
	}
	page, ok := res.(pageResponse[api.Domain])
	if !ok {
		return Page[api.Domain]{}, problemError(res)
	}
	next, _ := NextPageToken(res)
	previous, _ := PreviousPageToken(res)
	last, _ := LastPageToken(res)
	return Page[api.Domain]{
		Items:         page.GetResponse(),
		NextToken:     next,
		PreviousToken: previous,
		LastToken:     last,
	}, nil
}

// Get returns one domain by identifier.
func (s *DomainsService) Get(ctx context.Context, domainID uuid.UUID) (*api.Domain, error) {
	res, err := s.client.GetDomain(ctx, api.GetDomainParams{
		Slug:     api.TenantSlug(s.slug),
		DomainID: domainID,
	})
	return domainResult(res, err)
}

// Create onboards a domain for outbound sending.
func (s *DomainsService) Create(ctx context.Context, input CreateDomainInput) (*api.Domain, error) {
	request := input.request()
	res, err := s.client.CreateDomain(ctx, &request, api.CreateDomainParams{Slug: api.TenantSlug(s.slug)})
	return domainResult(res, err)
}

// Replace replaces the writable configuration of a domain.
//
// The v2 contract currently exposes only the DMARC report address as writable
// domain configuration. A nil dmarcReportAddress clears the stored address. Use
// a pointer to an empty string only when the control plane should store an
// empty value. Use the embedded generated client for fields added to the
// contract before this facade is updated.
func (s *DomainsService) Replace(ctx context.Context, domainID uuid.UUID, dmarcReportAddress *string) (*api.Domain, error) {
	var request api.ReplaceDomainRequest
	if dmarcReportAddress == nil {
		request.DmarcReportAddress.SetToNull()
	} else {
		request.DmarcReportAddress.SetTo(*dmarcReportAddress)
	}
	res, err := s.client.ReplaceDomain(ctx, &request, api.ReplaceDomainParams{
		Slug:     api.TenantSlug(s.slug),
		DomainID: domainID,
	})
	return domainResult(res, err)
}

// Delete deletes a domain and queues external cleanup.
func (s *DomainsService) Delete(ctx context.Context, domainID uuid.UUID) error {
	res, err := s.client.DeleteDomain(ctx, api.DeleteDomainParams{
		Slug:     api.TenantSlug(s.slug),
		DomainID: domainID,
	})
	if err != nil {
		return err
	}
	if _, ok := res.(*api.DeleteDomainNoContent); ok {
		return nil
	}
	return problemError(res)
}

// ListListenerGrants returns a lazy sequence of the listeners authorized to
// send from a domain.
func (s *DomainsService) ListListenerGrants(ctx context.Context, domainID uuid.UUID, opts ListOptions) iter.Seq2[api.DomainListenerGrant, error] {
	params := api.ListDomainListenerGrantsParams{
		Slug:     api.TenantSlug(s.slug),
		DomainID: domainID,
	}
	if err := opts.apply(&params.PageSize); err != nil {
		return errorSeq[api.DomainListenerGrant](err)
	}
	return paginate(ctx, opts.PageToken, func(ctx context.Context, token string) ([]api.DomainListenerGrant, string, error) {
		page, err := s.fetchListenerGrantPage(ctx, params, token)
		if err != nil {
			return nil, "", err
		}
		return page.Items, page.NextToken, nil
	})
}

// ListenerGrantPages returns a page-level iterator over a domain's listener
// grants.
//
// The first page is fetched when Next is called. Use [Pager.Page] for the
// cursor metadata of the most recently loaded page.
func (s *DomainsService) ListenerGrantPages(domainID uuid.UUID, opts ListOptions) *Pager[api.DomainListenerGrant] {
	params := api.ListDomainListenerGrantsParams{
		Slug:     api.TenantSlug(s.slug),
		DomainID: domainID,
	}
	if err := opts.apply(&params.PageSize); err != nil {
		return errorPager[api.DomainListenerGrant](err)
	}
	return newPager(func(ctx context.Context, token string) (Page[api.DomainListenerGrant], error) {
		return s.fetchListenerGrantPage(ctx, params, token)
	})
}

func (s *DomainsService) fetchListenerGrantPage(ctx context.Context, params api.ListDomainListenerGrantsParams, token string) (Page[api.DomainListenerGrant], error) {
	pageParams := params
	if token != "" {
		pageParams.PageToken = api.NewOptString(token)
	}
	res, err := s.client.ListDomainListenerGrants(ctx, pageParams)
	if err != nil {
		return Page[api.DomainListenerGrant]{}, err
	}
	page, ok := res.(pageResponse[api.DomainListenerGrant])
	if !ok {
		return Page[api.DomainListenerGrant]{}, problemError(res)
	}
	next, _ := NextPageToken(res)
	previous, _ := PreviousPageToken(res)
	last, _ := LastPageToken(res)
	return Page[api.DomainListenerGrant]{
		Items:         page.GetResponse(),
		NextToken:     next,
		PreviousToken: previous,
		LastToken:     last,
	}, nil
}

// domainResult unwraps the success variants used by domain operations.
func domainResult(res any, err error) (*api.Domain, error) {
	if err != nil {
		return nil, err
	}
	switch value := res.(type) {
	case *api.GetDomainOKHeaders:
		return &value.Response, nil
	case *api.CreateDomainOK:
		return &value.Response, nil
	case *api.CreateDomainCreated:
		return &value.Response, nil
	case *api.CreateDomainAccepted:
		return &value.Response, nil
	}
	return nil, problemError(res)
}
