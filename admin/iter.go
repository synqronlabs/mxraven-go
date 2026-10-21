package admin

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/synqronlabs/mxraven-go/admin/api"
)

const (
	// MaxPageSize is the largest page size accepted by the control plane.
	MaxPageSize = 500
	// MinSearchLength is the shortest accepted search term.
	MinSearchLength = 3
	// MaxSearchLength is the longest accepted search term.
	MaxSearchLength = 200
)

// ListOptions are the options common to every paginated list operation.
//
// The zero value requests the first page with the server's default page size.
// A zero PageSize selects that default.
type ListOptions struct {
	// PageSize is the maximum number of items to request per page, between 1
	// and [MaxPageSize]. Zero uses the server default.
	PageSize int
	// PageToken resumes iteration from an opaque token returned by a previous
	// page. Empty starts at the first page.
	PageToken string
}

// apply validates the options and copies the page size into params.
func (o ListOptions) apply(pageSize *api.OptInt) error {
	if o.PageSize < 0 || o.PageSize > MaxPageSize {
		return fmt.Errorf("admin: page size must be between 1 and %d, or zero for the default", MaxPageSize)
	}
	if utf8.RuneCountInString(o.PageToken) > 2048 {
		return errors.New("admin: page token is too long")
	}
	if o.PageSize > 0 {
		*pageSize = api.NewOptInt(o.PageSize)
	}
	return nil
}

// normalizeSearch trims and validates a search term.
func normalizeSearch(search string) (string, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return "", nil
	}
	length := utf8.RuneCountInString(search)
	if length < MinSearchLength {
		return "", fmt.Errorf("admin: search must be at least %d characters", MinSearchLength)
	}
	if length > MaxSearchLength {
		return "", fmt.Errorf("admin: search must be at most %d characters", MaxSearchLength)
	}
	return search, nil
}

// normalizeRequiredSearch trims and validates a required search term.
func normalizeRequiredSearch(search string) (string, error) {
	search, err := normalizeSearch(search)
	if err != nil {
		return "", err
	}
	if search == "" {
		return "", errors.New("admin: search is required")
	}
	return search, nil
}

// pageResponse is implemented by generated list response wrappers.
type pageResponse[T any] interface {
	GetResponse() []T
}

// paginate yields items lazily across cursor pages.
func paginate[T any](
	ctx context.Context,
	start string,
	fetch func(ctx context.Context, token string) (items []T, next string, err error),
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		token := start
		seen := make(map[string]struct{})
		if start != "" {
			seen[start] = struct{}{}
		}
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}
			items, next, err := fetch(ctx, token)
			if err != nil {
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if next == "" {
				return
			}
			if _, ok := seen[next]; ok {
				yield(zero, fmt.Errorf("admin: control plane returned a repeated page token"))
				return
			}
			seen[next] = struct{}{}
			token = next
		}
	}
}

// errorSeq yields one error and no items.
func errorSeq[T any](err error) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		yield(zero, err)
	}
}

// Take reads at most n items from seq.
//
// It fetches only the pages needed to satisfy the limit. When iteration fails,
// Take returns the items read so far together with the error. A non-positive
// limit returns no items and does not start iteration.
func Take[T any](seq iter.Seq2[T, error], n int) ([]T, error) {
	if seq == nil {
		return nil, errors.New("admin: nil sequence")
	}
	if n <= 0 {
		return nil, nil
	}
	items := make([]T, 0, min(n, 16))
	for item, err := range seq {
		if err != nil {
			return items, err
		}
		items = append(items, item)
		if len(items) >= n {
			break
		}
	}
	return items, nil
}

// Collect reads every item from seq.
//
// When iteration fails, Collect returns the items read so far together with the
// error. Prefer [Take] when the collection may be large.
func Collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	if seq == nil {
		return nil, errors.New("admin: nil sequence")
	}
	var items []T
	for item, err := range seq {
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
	return items, nil
}
