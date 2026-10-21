package admin

import (
	"context"
	"iter"

	"github.com/google/uuid"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

// ListListenersOptions filters and paginates [ListenersService.List].
//
// The zero value lists every listener with the server's default page size.
type ListListenersOptions struct {
	ListOptions
	// Search is a case-insensitive search across listener fields. It must be at
	// least [MinSearchLength] characters when non-empty.
	Search string
	// ListenerType restricts results to one listener kind.
	ListenerType api.ListenerType
	// StreamType restricts results to one mail stream.
	StreamType api.ListenerStreamType
	// DefaultTerminalActionType restricts results to one fallback action.
	DefaultTerminalActionType api.DefaultTerminalActionType
	// RspamdScanningEnabled restricts results by inbound scanning state.
	RspamdScanningEnabled *bool
}

// apply validates the options and copies them into params.
func (o ListListenersOptions) apply(params *api.ListListenersParams) error {
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
	if o.ListenerType != "" {
		params.ListenerType = api.NewOptListenerType(o.ListenerType)
	}
	if o.StreamType != "" {
		params.StreamType = api.NewOptListenerStreamType(o.StreamType)
	}
	if o.DefaultTerminalActionType != "" {
		params.DefaultTerminalActionType = api.NewOptDefaultTerminalActionType(o.DefaultTerminalActionType)
	}
	if o.RspamdScanningEnabled != nil {
		params.RspamdScanningEnabled = api.NewOptBool(*o.RspamdScanningEnabled)
	}
	return nil
}

// CreateListenerInput describes a listener to create.
//
// A nil DefaultTerminalActionPayload omits the payload field. A nil
// RspamdScanningEnabled lets the control plane apply its listener-type default.
type CreateListenerInput struct {
	// DisplayName is the human-readable listener name.
	DisplayName string
	// ListenerType selects a submission or MTA listener.
	ListenerType api.ListenerType
	// StreamType is the mail stream the listener serves. The system stream
	// cannot be created through the API.
	StreamType api.CreatableListenerStreamType
	// DefaultTerminalActionType is the fallback action for unmatched mail.
	DefaultTerminalActionType api.DefaultTerminalActionType
	// DefaultTerminalActionPayload is action-specific data. Nil omits it.
	DefaultTerminalActionPayload api.DefaultTerminalActionPayload
	// RspamdScanningEnabled overrides the listener-type default when non-nil.
	RspamdScanningEnabled *bool
}

// request converts the input to the generated request model.
func (in CreateListenerInput) request() api.CreateListenerRequest {
	request := api.CreateListenerRequest{
		DisplayName:               in.DisplayName,
		ListenerType:              in.ListenerType,
		StreamType:                in.StreamType,
		DefaultTerminalActionType: in.DefaultTerminalActionType,
	}
	if in.DefaultTerminalActionPayload != nil {
		request.DefaultTerminalActionPayload = api.NewOptDefaultTerminalActionPayload(in.DefaultTerminalActionPayload)
	}
	if in.RspamdScanningEnabled != nil {
		request.RspamdScanningEnabled = api.NewOptBool(*in.RspamdScanningEnabled)
	}
	return request
}

// ListenersService manages the submission and MTA listeners of one tenant.
//
// Obtain one from [Workspace]. It is safe for concurrent use.
type ListenersService struct {
	client *Client
	slug   string
}

// List returns a lazy sequence of listeners.
//
// Pages are fetched only as the sequence is consumed. A validation, transport,
// or API error is yielded once and stops iteration.
func (s *ListenersService) List(ctx context.Context, opts ListListenersOptions) iter.Seq2[api.Listener, error] {
	params := api.ListListenersParams{Slug: api.TenantSlug(s.slug)}
	if err := opts.apply(&params); err != nil {
		return errorSeq[api.Listener](err)
	}
	return paginate(ctx, opts.PageToken, func(ctx context.Context, token string) ([]api.Listener, string, error) {
		page, err := s.fetchPage(ctx, params, token)
		if err != nil {
			return nil, "", err
		}
		return page.Items, page.NextToken, nil
	})
}

// Pages returns a page-level iterator over listeners.
//
// The first page is fetched when Next is called. Use [Pager.Page] for the
// cursor metadata of the most recently loaded page.
func (s *ListenersService) Pages(opts ListListenersOptions) *Pager[api.Listener] {
	params := api.ListListenersParams{Slug: api.TenantSlug(s.slug)}
	if err := opts.apply(&params); err != nil {
		return errorPager[api.Listener](err)
	}
	return newPager(func(ctx context.Context, token string) (Page[api.Listener], error) {
		return s.fetchPage(ctx, params, token)
	})
}

func (s *ListenersService) fetchPage(ctx context.Context, params api.ListListenersParams, token string) (Page[api.Listener], error) {
	pageParams := params
	if token != "" {
		pageParams.PageToken = api.NewOptString(token)
	}
	res, err := s.client.ListListeners(ctx, pageParams)
	if err != nil {
		return Page[api.Listener]{}, err
	}
	page, ok := res.(pageResponse[api.Listener])
	if !ok {
		return Page[api.Listener]{}, problemError(res)
	}
	next, _ := NextPageToken(res)
	previous, _ := PreviousPageToken(res)
	last, _ := LastPageToken(res)
	return Page[api.Listener]{
		Items:         page.GetResponse(),
		NextToken:     next,
		PreviousToken: previous,
		LastToken:     last,
	}, nil
}

// Get returns one listener by identifier.
func (s *ListenersService) Get(ctx context.Context, listenerID uuid.UUID) (*api.Listener, error) {
	res, err := s.client.GetListener(ctx, api.GetListenerParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return listenerResult(res, err)
}

// Create creates a listener.
func (s *ListenersService) Create(ctx context.Context, input CreateListenerInput) (*api.Listener, error) {
	request := input.request()
	res, err := s.client.CreateListener(ctx, &request, api.CreateListenerParams{Slug: api.TenantSlug(s.slug)})
	return listenerResult(res, err)
}

// Update replaces the writable listener configuration.
func (s *ListenersService) Update(ctx context.Context, listenerID uuid.UUID, request api.UpdateListenerRequest) (*api.Listener, error) {
	res, err := s.client.UpdateListener(ctx, &request, api.UpdateListenerParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return listenerResult(res, err)
}

// Rename changes a listener's display name.
//
// It is a convenience wrapper around [ListenersService.Update].
func (s *ListenersService) Rename(ctx context.Context, listenerID uuid.UUID, displayName string) (*api.Listener, error) {
	return s.Update(ctx, listenerID, api.UpdateListenerRequest{DisplayName: displayName})
}

// Delete deletes a listener.
func (s *ListenersService) Delete(ctx context.Context, listenerID uuid.UUID) error {
	res, err := s.client.DeleteListener(ctx, api.DeleteListenerParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	if err != nil {
		return err
	}
	if _, ok := res.(*api.DeleteListenerNoContent); ok {
		return nil
	}
	return problemError(res)
}

// SetDefaultTerminalAction replaces a listener's fallback terminal action.
//
// A nil payload omits the action payload. Use [DeliverPayload],
// [DeliverDedicatedPayload], [RelayPayload], [AutoReplyPayload], [DropPayload],
// or [RejectPayload] to build one.
func (s *ListenersService) SetDefaultTerminalAction(
	ctx context.Context,
	listenerID uuid.UUID,
	actionType api.DefaultTerminalActionType,
	payload api.DefaultTerminalActionPayload,
) (*api.Listener, error) {
	request := api.UpdateListenerDefaultTerminalActionRequest{ActionType: actionType}
	if payload != nil {
		request.ActionPayload = api.NewOptDefaultTerminalActionPayload(payload)
	}
	res, err := s.client.UpdateListenerDefaultTerminalAction(ctx, &request, api.UpdateListenerDefaultTerminalActionParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return listenerResult(res, err)
}

// SetRspamdScanning enables or disables inbound Rspamd scanning for a listener.
func (s *ListenersService) SetRspamdScanning(ctx context.Context, listenerID uuid.UUID, enabled bool) (*api.Listener, error) {
	res, err := s.client.UpdateListenerRspamdScanning(ctx, &api.UpdateListenerRspamdScanningRequest{Enabled: enabled}, api.UpdateListenerRspamdScanningParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return listenerResult(res, err)
}

// GetNotifications returns a listener's client-notification setting.
func (s *ListenersService) GetNotifications(ctx context.Context, listenerID uuid.UUID) (*api.ListenerNotificationSetting, error) {
	res, err := s.client.GetListenerNotifications(ctx, api.GetListenerNotificationsParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return notificationResult(res, err)
}

// SetNotifications enables or disables client notifications for a listener.
func (s *ListenersService) SetNotifications(ctx context.Context, listenerID uuid.UUID, enabled bool) (*api.ListenerNotificationSetting, error) {
	res, err := s.client.UpdateListenerNotifications(ctx, &api.UpdateListenerNotificationsRequest{Enabled: enabled}, api.UpdateListenerNotificationsParams{
		Slug:       api.TenantSlug(s.slug),
		ListenerID: listenerID,
	})
	return notificationResult(res, err)
}

// listenerResult unwraps the success variants used by listener operations.
func listenerResult(res any, err error) (*api.Listener, error) {
	if err != nil {
		return nil, err
	}
	switch value := res.(type) {
	case *api.ListenerHeaders:
		return &value.Response, nil
	case *api.GetListenerOKHeaders:
		return &value.Response, nil
	}
	return nil, problemError(res)
}

// notificationResult unwraps listener notification responses.
func notificationResult(res any, err error) (*api.ListenerNotificationSetting, error) {
	if err != nil {
		return nil, err
	}
	if value, ok := res.(*api.ListenerNotificationSettingHeaders); ok {
		return &value.Response, nil
	}
	return nil, problemError(res)
}
