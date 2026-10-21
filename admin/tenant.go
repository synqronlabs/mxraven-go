package admin

import (
	"context"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// TenantService reads the identity and provisioning state of a tenant.
//
// Obtain one from [Workspace]. It is safe for concurrent use.
type TenantService struct {
	client *Client
	slug   string
}

// Get returns the tenant record for the workspace.
func (s *TenantService) Get(ctx context.Context) (*api.Tenant, error) {
	res, err := s.client.GetTenant(ctx, api.GetTenantParams{Slug: api.TenantSlug(s.slug)})
	tenant, err := Do[api.Tenant](res, err)
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}
