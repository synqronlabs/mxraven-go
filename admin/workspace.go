package admin

import "strings"

// Workspace is a tenant-bound view of the control plane.
//
// A workspace binds one tenant slug so operations do not repeat it. Obtain one
// from [Client.Workspace]. A workspace is a lightweight binding; treat it and
// its services as read-only. It is safe for concurrent use.
//
// Workspace embeds [WorkspaceServices], so every resource family is available
// as a promoted field:
//
//	ws := client.Workspace("acme")
//	tenant, err := ws.Tenant.Get(ctx)
//	domains, err := admin.Collect(ws.Domains.List(ctx, admin.ListDomainsOptions{}))
type Workspace struct {
	*WorkspaceServices
	slug string
}

// Workspace returns a tenant-bound view of the control plane.
//
// The slug is trimmed of surrounding whitespace. An empty slug is not rejected
// locally; operations that use it will fail with the control plane's response.
func (c *Client) Workspace(slug string) *Workspace {
	slug = strings.TrimSpace(slug)
	return &Workspace{
		WorkspaceServices: newWorkspaceServices(c, slug),
		slug:              slug,
	}
}

// Slug returns the tenant slug this workspace is bound to.
func (w *Workspace) Slug() string {
	return w.slug
}
