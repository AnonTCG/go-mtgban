package cardtrader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestBlueprintsForGameIDOrder pins that a pooled fetch returns the
// blueprints in catalog order however the expansions finish, and skips the
// one that fails.
func TestBlueprintsForGameIDOrder(t *testing.T) {
	expansions := `[{"id":1,"game_id":26,"name":"One"},{"id":2,"game_id":26,"name":"Two"},` +
		`{"id":9,"game_id":1,"name":"Magic"},{"id":3,"game_id":26,"name":"Three"},` +
		`{"id":4,"game_id":26,"name":"Four"},{"id":5,"game_id":26,"name":"Five"}]`

	// The first expansion answers only once the third has, so the two
	// finish out of catalog order.
	thirdServed := make(chan struct{})
	transport := transportFunc(func(req *http.Request) (*http.Response, error) {
		body := expansions
		id := req.URL.Query().Get("expansion_id")
		switch id {
		case "":
		case "1":
			<-thirdServed
			body = `[{"id":10},{"id":11}]`
		case "3":
			defer close(thirdServed)
			body = `[{"id":30},{"id":31}]`
		case "4":
			body = `{"extra":{"message":"boom"}}`
		default:
			body = fmt.Sprintf(`[{"id":%s0},{"id":%s1}]`, id, id)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	client := &CTAuthClient{client: &http.Client{Transport: transport}}

	var mu sync.Mutex
	var logs []string
	logf := func(format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, a...))
	}

	blueprints, _, err := BlueprintsForGameID(context.Background(), client, GamePalworld, "", 4, logf)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, bp := range blueprints {
		ids = append(ids, bp.ID)
	}
	want := []int{10, 11, 20, 21, 30, 31, 50, 51}
	if !slices.Equal(ids, want) {
		t.Errorf("got blueprints %v, want %v", ids, want)
	}
	wantLogs := []string{"skipping 4 Four due to boom"}
	if !slices.Equal(logs, wantLogs) {
		t.Errorf("got logs %q, want %q", logs, wantLogs)
	}
}
