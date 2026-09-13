package admin_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/synqronlabs/mxraven-go/admin"
)

func TestPagerIteratesLazily(t *testing.T) {
	pages := []admin.Page[string]{
		{Items: []string{"a", "b"}, NextToken: "t1"},
		{Items: []string{"c"}, NextToken: ""},
	}
	wantTokens := []string{"", "t1"}

	var calls int
	pager, err := admin.NewPager(func(_ context.Context, token string) (admin.Page[string], error) {
		if calls >= len(pages) {
			t.Fatalf("unexpected fetch call %d", calls)
		}
		if token != wantTokens[calls] {
			t.Errorf("fetch token = %q, want %q", token, wantTokens[calls])
		}
		page := pages[calls]
		calls++
		return page, nil
	})
	if err != nil {
		t.Fatalf("NewPager() error: %v", err)
	}

	var got []string
	for pager.Next(context.Background()) {
		got = append(got, pager.Items()...)
	}
	if err := pager.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if calls != len(pages) {
		t.Errorf("fetch calls = %d, want %d", calls, len(pages))
	}
	if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
	if pager.Token() != "" {
		t.Errorf("Token() = %q, want empty after last page", pager.Token())
	}
}

func TestPagerStopsEarly(t *testing.T) {
	var calls int
	pager, err := admin.NewPager(func(_ context.Context, _ string) (admin.Page[int], error) {
		calls++
		return admin.Page[int]{Items: []int{calls}, NextToken: "more"}, nil
	})
	if err != nil {
		t.Fatalf("NewPager() error: %v", err)
	}

	if !pager.Next(context.Background()) {
		t.Fatal("Next() = false, want true")
	}
	if got := pager.Items(); !slices.Equal(got, []int{1}) {
		t.Errorf("items = %v, want [1]", got)
	}
	if calls != 1 {
		t.Errorf("fetch calls = %d, want 1 (must not prefetch)", calls)
	}
	if pager.Token() != "more" {
		t.Errorf("Token() = %q, want %q", pager.Token(), "more")
	}
}

func TestPagerError(t *testing.T) {
	wantErr := errors.New("rate limited")
	var calls int
	pager, err := admin.NewPager(func(_ context.Context, _ string) (admin.Page[string], error) {
		calls++
		if calls == 2 {
			return admin.Page[string]{}, wantErr
		}
		return admin.Page[string]{Items: []string{"a"}, NextToken: "t1"}, nil
	})
	if err != nil {
		t.Fatalf("NewPager() error: %v", err)
	}

	ctx := context.Background()
	if !pager.Next(ctx) {
		t.Fatal("first Next() = false, want true")
	}
	if pager.Next(ctx) {
		t.Fatal("second Next() = true, want false")
	}
	if !errors.Is(pager.Err(), wantErr) {
		t.Errorf("Err() = %v, want %v", pager.Err(), wantErr)
	}
	if pager.Next(ctx) {
		t.Error("Next() after error = true, want false")
	}
}

func TestNewPagerRejectsNilFetcher(t *testing.T) {
	if _, err := admin.NewPager[string](nil); err == nil {
		t.Fatal("NewPager(nil) error = nil, want error")
	}
}
