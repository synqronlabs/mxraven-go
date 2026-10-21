package admin_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/synqronlabs/mxraven-go/admin"
)

func TestTakeAndCollect(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		for i := 1; i <= 5; i++ {
			if !yield(i, nil) {
				return
			}
		}
	}

	items, err := admin.Take[int](seq, 2)
	if err != nil {
		t.Fatalf("Take() error: %v", err)
	}
	if want := []int{1, 2}; !slices.Equal(items, want) {
		t.Errorf("Take() = %v, want %v", items, want)
	}

	items, err = admin.Collect[int](seq)
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}
	if want := []int{1, 2, 3, 4, 5}; !slices.Equal(items, want) {
		t.Errorf("Collect() = %v, want %v", items, want)
	}
}

func TestTakeStopsOnError(t *testing.T) {
	wantErr := errors.New("boom")
	seq := func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		yield(0, wantErr)
	}

	items, err := admin.Take[int](seq, 10)
	if !errors.Is(err, wantErr) {
		t.Errorf("Take() error = %v, want %v", err, wantErr)
	}
	if want := []int{1}; !slices.Equal(items, want) {
		t.Errorf("Take() = %v, want %v", items, want)
	}
}

func TestTakeNonPositiveDoesNotIterate(t *testing.T) {
	called := false
	seq := func(func(int, error) bool) {
		called = true
	}
	items, err := admin.Take[int](seq, 0)
	if err != nil {
		t.Fatalf("Take() error: %v", err)
	}
	if items != nil {
		t.Errorf("Take() = %v, want nil", items)
	}
	if called {
		t.Error("Take(0) iterated the sequence, want no iteration")
	}
}

func TestTakeAndCollectRejectNil(t *testing.T) {
	if _, err := admin.Take[int](nil, 1); err == nil {
		t.Error("Take(nil) error = nil, want error")
	}
	if _, err := admin.Collect[int](nil); err == nil {
		t.Error("Collect(nil) error = nil, want error")
	}
}
