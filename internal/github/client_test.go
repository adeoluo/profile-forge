package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adeoluo/profile-forge/internal/fakegh"
)

func newTestClient(base string, cache *Cache) *Client {
	c := NewClient("", cache)
	c.BaseURL = base
	c.Backoff = time.Millisecond
	return c
}

func TestCacheRoundTripAndTTL(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCache(t.TempDir(), time.Hour)
	c.Now = func() time.Time { return now }
	if _, err := c.Get("u", false); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("empty cache: %v", err)
	}
	e := &Entry{URL: "u", Status: 200, Body: []byte(`{"a":1}`), FetchedAt: now.Add(-30 * time.Minute)}
	if err := c.Put(e); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		age        time.Duration
		allowStale bool
		hit        bool
	}{
		{"fresh", 30 * time.Minute, false, true},
		{"expired", 2 * time.Hour, false, false},
		{"expired but stale allowed", 2 * time.Hour, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.Now = func() time.Time { return e.FetchedAt.Add(tc.age) }
			got, err := c.Get("u", tc.allowStale)
			if tc.hit {
				if err != nil || string(got.Body) != `{"a":1}` || got.Status != 200 {
					t.Fatalf("want hit, got %v %v", got, err)
				}
			} else if !errors.Is(err, ErrCacheMiss) {
				t.Fatalf("want miss, got %v", err)
			}
		})
	}
}

func TestCacheCorruptAndCollision(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, time.Hour)
	if err := os.WriteFile(c.path("bad"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("bad", true); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt: %v", err)
	}
	// An entry whose recorded URL differs from the requested one is a miss.
	if err := os.WriteFile(c.path("other"), []byte(`{"url":"different","status":200}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("other", true); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("collision: %v", err)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".entry-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestCachePutFailsOnUnwritableDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewCache(filepath.Join(f, "sub"), time.Hour)
	if err := c.Put(&Entry{URL: "u"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDefaultDir(t *testing.T) {
	if !strings.HasSuffix(DefaultDir(), "profile-forge") {
		t.Fatal(DefaultDir())
	}
}

func TestNextPage(t *testing.T) {
	cases := map[string]string{
		"": "",
		`<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"`:  "https://api.github.com/x?page=2",
		`<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=1>; rel="first"`: "",
	}
	for in, want := range cases {
		if got := NextPage(in); got != want {
			t.Errorf("NextPage(%q) = %q want %q", in, got, want)
		}
	}
}

func TestListReposPaginatesAndFilters(t *testing.T) {
	srv := fakegh.New("tester", fakegh.Sample())
	defer srv.Close()
	c := newTestClient(srv.URL, nil)
	repos, err := c.ListRepos(context.Background(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range repos {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, ","); got != "alpha,beta,delta,forked,gamma,old,stray" {
		t.Fatalf("names = %s", got)
	}
	// 8 repos at page size 2 is 4 pages.
	if got := c.Stats().Requests; got != 4 {
		t.Errorf("requests = %d want 4", got)
	}
	if _, err := c.ListRepos(context.Background(), "nobody"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown user: %v", err)
	}
}

func TestFetchAllEnrichesAndCaches(t *testing.T) {
	srv := fakegh.New("tester", fakegh.Sample())
	defer srv.Close()
	cache := NewCache(t.TempDir(), time.Hour)
	c := newTestClient(srv.URL, cache)
	c.Token = "secret"
	repos, err := c.FetchAll(context.Background(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Repo{}
	for _, r := range repos {
		by[r.Name] = r
	}
	if by["alpha"].LatestRelease != "v1.2.0" || !strings.HasSuffix(by["alpha"].ReleaseURL, "/releases/tag/v1.2.0") {
		t.Errorf("alpha release: %+v", by["alpha"])
	}
	if by["beta"].LatestRelease != "" || by["beta"].Languages["Go"] != 5000 {
		t.Errorf("beta: %+v", by["beta"])
	}
	if by["delta"].Description != "" || by["delta"].Language != "Rust" {
		t.Errorf("delta: %+v", by["delta"])
	}
	if got := srv.AuthHeader.Load(); got != "Bearer secret" {
		t.Errorf("auth header = %v", got)
	}
	first := c.Stats()
	if first.Requests != 4+2*7 || first.CacheHits != 0 {
		t.Fatalf("first run stats = %+v", first)
	}

	// Second run within TTL is served entirely from cache.
	c2 := newTestClient(srv.URL, cache)
	if _, err := c2.FetchAll(context.Background(), "tester"); err != nil {
		t.Fatal(err)
	}
	if s := c2.Stats(); s.Requests != 0 || s.CacheHits != first.Requests {
		t.Fatalf("cached run stats = %+v", s)
	}

	// Offline works after the server is gone, even when entries are stale.
	srv.Close()
	cache.Now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	c3 := newTestClient(srv.URL, cache)
	c3.Offline = true
	got, err := c3.FetchAll(context.Background(), "tester")
	if err != nil || len(got) != len(repos) {
		t.Fatalf("offline: %v (%d repos)", err, len(got))
	}
	// Online with an expired cache and a dead server fails.
	c4 := newTestClient(srv.URL, cache)
	c4.Retries = 0
	if _, err := c4.FetchAll(context.Background(), "tester"); err == nil {
		t.Fatal("expected network error")
	}
}

func TestOfflineMissIsAnError(t *testing.T) {
	c := newTestClient("http://127.0.0.1:1", NewCache(t.TempDir(), time.Hour))
	c.Offline = true
	_, err := c.ListRepos(context.Background(), "tester")
	if err == nil || !strings.Contains(err.Error(), "offline mode") {
		t.Fatalf("got %v", err)
	}
	c.Cache = nil
	if _, err := c.Languages(context.Background(), "a", "b"); err == nil {
		t.Fatal("offline without cache must fail")
	}
}

func TestErrorsAndRetries(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "/flaky/"):
			if n%2 == 1 {
				http.Error(w, `{"message":"boom"}`, http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"Go": 10}`))
		case strings.Contains(r.URL.Path, "/down/"):
			http.Error(w, "plain failure", http.StatusServiceUnavailable)
		case strings.Contains(r.URL.Path, "/limited/"):
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
		case strings.Contains(r.URL.Path, "/teapot/"):
			http.Error(w, `{"message":"short and stout"}`, http.StatusTeapot)
		case strings.Contains(r.URL.Path, "/garbage/"):
			_, _ = w.Write([]byte(`not json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c := newTestClient(srv.URL, nil)

	langs, err := c.Languages(ctx, "o", "flaky")
	if err != nil || langs["Go"] != 10 {
		t.Fatalf("flaky: %v %v", langs, err)
	}
	_, err = c.Languages(ctx, "o", "down")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 503 || apiErr.Message != "plain failure" {
		t.Fatalf("down: %v", err)
	}
	_, err = c.Languages(ctx, "o", "limited")
	if err == nil || !strings.Contains(err.Error(), "set GITHUB_TOKEN") {
		t.Fatalf("limited: %v", err)
	}
	if _, err = c.Languages(ctx, "o", "teapot"); err == nil || !strings.Contains(err.Error(), "HTTP 418: short and stout") {
		t.Fatalf("teapot: %v", err)
	}
	if _, err = c.Languages(ctx, "o", "garbage"); err == nil || !strings.Contains(err.Error(), "decode languages") {
		t.Fatalf("garbage: %v", err)
	}
	if _, _, err = c.LatestRelease(ctx, "o", "garbage"); err == nil || !strings.Contains(err.Error(), "decode release") {
		t.Fatalf("garbage release: %v", err)
	}
	if m, err := c.Languages(ctx, "o", "missing"); err != nil || len(m) != 0 {
		t.Fatalf("missing languages should be empty: %v %v", m, err)
	}
	if _, err := c.ListRepos(ctx, "garbage"); err == nil {
		t.Fatal("garbage repo list should fail")
	}
}

func TestFetchAllPropagatesFirstError(t *testing.T) {
	srv := fakegh.New("tester", fakegh.Sample())
	defer srv.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/languages") {
			http.Error(w, `{"message":"nope"}`, http.StatusUnauthorized)
			return
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer broken.Close()
	c := newTestClient(broken.URL, nil)
	c.Concurrency = 0 // clamps to one worker
	_, err := c.FetchAll(context.Background(), "tester")
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("got %v", err)
	}
}

func TestFetchAllHonorsCancellation(t *testing.T) {
	srv := fakegh.New("tester", fakegh.Sample())
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient(srv.URL, nil)
	c.Retries = 0
	if _, err := c.FetchAll(ctx, "tester"); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestResolveToken(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	gh := func(tok string, err error) func() (string, error) { return func() (string, error) { return tok, err } }
	cases := []struct {
		name string
		env  map[string]string
		gh   func() (string, error)
		want string
	}{
		{"github token wins", map[string]string{"GITHUB_TOKEN": " a ", "GH_TOKEN": "b"}, gh("c", nil), "a"},
		{"gh token env", map[string]string{"GH_TOKEN": "b"}, gh("c", nil), "b"},
		{"gh cli", nil, gh("c\n", nil), "c"},
		{"gh cli fails", nil, gh("", errors.New("not logged in")), ""},
		{"no gh", nil, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveToken(env(tc.env), tc.gh); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestAPIMessageTruncates(t *testing.T) {
	long := strings.Repeat("x", 500)
	if got := apiMessage([]byte(long)); len(got) != 200 {
		t.Fatalf("len = %d", len(got))
	}
}
