package tcgplayer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

// WorkerPool only logs a page's error; loadPages is what turns one into a
// failed Load rather than an inventory missing that page.
func TestLoadPagesFailsWhenAPageDoes(t *testing.T) {
	var mtx sync.Mutex
	var pages []int
	var consumed int
	run := func(failAt int) error {
		pages, consumed = nil, 0
		return loadPages(context.Background(), 2, 600,
			func(ctx context.Context, page int, channel chan<- genericChan) error {
				mtx.Lock()
				pages = append(pages, page)
				mtx.Unlock()
				if page == failAt {
					return errors.New("page refused")
				}
				channel <- genericChan{key: "k"}
				return nil
			},
			func(genericChan) { consumed++ },
			func(string, ...any) {},
		)
	}

	if err := run(-1); err != nil {
		t.Errorf("loadPages() with every page good error = %v, want nil", err)
	}
	slices.Sort(pages)
	if !slices.Equal(pages, []int{0, 250, 500}) || consumed != 3 {
		t.Errorf("loadPages() visited %v and consumed %d, want [0 250 500] and 3", pages, consumed)
	}

	err := run(250)
	if err == nil || !strings.Contains(err.Error(), "1 of 3 pages") {
		t.Errorf("loadPages() with a failed page error = %v, want 1 of 3 pages reported", err)
	}
}

func TestLoadPagesFailsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := loadPages(ctx, 2, 250,
		func(ctx context.Context, page int, channel chan<- genericChan) error {
			return nil
		},
		func(genericChan) {},
		func(string, ...any) {},
	)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("loadPages() on a cancelled context error = %v, want context.Canceled", err)
	}
}
