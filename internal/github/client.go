// Package github is a small GitHub REST API client with an on-disk cache.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultBaseURL is the public GitHub REST API endpoint.
const DefaultBaseURL = "https://api.github.com"

// Repo is the normalized metadata profile-forge uses for one repository.
type Repo struct {
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	HTMLURL       string           `json:"html_url"`
	Language      string           `json:"language"`
	Languages     map[string]int64 `json:"languages"`
	Topics        []string         `json:"topics"`
	Stars         int              `json:"stars"`
	PushedAt      time.Time        `json:"pushed_at"`
	LatestRelease string           `json:"latest_release,omitempty"`
	ReleaseURL    string           `json:"release_url,omitempty"`
	Archived      bool             `json:"archived"`
	Fork          bool             `json:"fork"`
}

// Stats counts network and cache activity for one run.
type Stats struct {
	Requests  int64 // HTTP requests sent
	CacheHits int64 // responses served from the cache
}

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL     string
	Token       string
	HTTP        *http.Client
	Cache       *Cache // optional
	Offline     bool   // serve only from cache, ignoring TTL
	Concurrency int
	UserAgent   string
	Retries     int
	Backoff     time.Duration

	requests  atomic.Int64
	cacheHits atomic.Int64
}

// NewClient returns a client with sensible defaults.
func NewClient(token string, cache *Cache) *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		Token:       token,
		HTTP:        &http.Client{Timeout: 30 * time.Second},
		Cache:       cache,
		Concurrency: 8,
		UserAgent:   "profile-forge",
		Retries:     2,
		Backoff:     500 * time.Millisecond,
	}
}

// Stats returns a snapshot of request counters.
func (c *Client) Stats() Stats {
	return Stats{Requests: c.requests.Load(), CacheHits: c.cacheHits.Load()}
}

// APIError is a non-success response from the API.
type APIError struct {
	URL     string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d: %s", e.URL, e.Status, e.Message)
}

func (c *Client) get(ctx context.Context, u string) (*Entry, error) {
	if c.Cache != nil {
		e, err := c.Cache.Get(u, c.Offline)
		if err == nil {
			c.cacheHits.Add(1)
			return e, nil
		}
		if !errors.Is(err, ErrCacheMiss) {
			return nil, err
		}
	}
	if c.Offline {
		return nil, fmt.Errorf("offline mode: no cached response for %s (run once online to fill the cache)", u)
	}
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.Backoff * time.Duration(attempt)):
			}
		}
		e, retry, err := c.fetch(ctx, u)
		if err == nil {
			if c.Cache != nil {
				if perr := c.Cache.Put(e); perr != nil {
					return nil, fmt.Errorf("write cache: %w", perr)
				}
			}
			return e, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) fetch(ctx context.Context, u string) (*Entry, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	c.requests.Add(1)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, true, err
	}
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound:
		return &Entry{
			URL:       u,
			Status:    resp.StatusCode,
			Link:      resp.Header.Get("Link"),
			FetchedAt: time.Now().UTC(),
			Body:      body,
		}, false, nil
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		msg := apiMessage(body)
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			msg += " (rate limit exhausted; set GITHUB_TOKEN or log in with gh auth login)"
		}
		return nil, false, &APIError{URL: u, Status: resp.StatusCode, Message: msg}
	case resp.StatusCode >= 500:
		return nil, true, &APIError{URL: u, Status: resp.StatusCode, Message: apiMessage(body)}
	default:
		return nil, false, &APIError{URL: u, Status: resp.StatusCode, Message: apiMessage(body)}
	}
}

func apiMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// NextPage extracts the rel="next" URL from a Link header.
func NextPage(link string) string {
	if m := nextLink.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

type apiRepo struct {
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	HTMLURL     string    `json:"html_url"`
	Language    *string   `json:"language"`
	Topics      []string  `json:"topics"`
	Stars       int       `json:"stargazers_count"`
	PushedAt    time.Time `json:"pushed_at"`
	Archived    bool      `json:"archived"`
	Fork        bool      `json:"fork"`
	Private     bool      `json:"private"`
}

// ListRepos returns every public repository owned by user, following pagination.
func (c *Client) ListRepos(ctx context.Context, user string) ([]Repo, error) {
	u := fmt.Sprintf("%s/users/%s/repos?per_page=100&type=owner&sort=full_name", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(user))
	var out []Repo
	for pages := 0; u != ""; pages++ {
		if pages >= 50 {
			return nil, errors.New("list repos: too many pages")
		}
		e, err := c.get(ctx, u)
		if err != nil {
			return nil, err
		}
		if e.Status == http.StatusNotFound {
			return nil, fmt.Errorf("user %q not found", user)
		}
		var page []apiRepo
		if err := json.Unmarshal(e.Body, &page); err != nil {
			return nil, fmt.Errorf("decode repos page: %w", err)
		}
		for _, r := range page {
			if r.Private {
				continue
			}
			out = append(out, Repo{
				Name:        r.Name,
				Description: deref(r.Description),
				HTMLURL:     r.HTMLURL,
				Language:    deref(r.Language),
				Topics:      r.Topics,
				Stars:       r.Stars,
				PushedAt:    r.PushedAt,
				Archived:    r.Archived,
				Fork:        r.Fork,
			})
		}
		u = NextPage(e.Link)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Languages returns bytes of code per language for one repository.
func (c *Client) Languages(ctx context.Context, owner, repo string) (map[string]int64, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/languages", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(owner), url.PathEscape(repo))
	e, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if e.Status == http.StatusNotFound {
		return map[string]int64{}, nil
	}
	m := map[string]int64{}
	if err := json.Unmarshal(e.Body, &m); err != nil {
		return nil, fmt.Errorf("decode languages for %s: %w", repo, err)
	}
	return m, nil
}

// LatestRelease returns the latest published release tag and URL, or empty
// strings when the repository has no release.
func (c *Client) LatestRelease(ctx context.Context, owner, repo string) (tag, htmlURL string, err error) {
	u := fmt.Sprintf("%s/repos/%s/%s/releases/latest", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(owner), url.PathEscape(repo))
	e, err := c.get(ctx, u)
	if err != nil {
		return "", "", err
	}
	if e.Status == http.StatusNotFound {
		return "", "", nil
	}
	var r struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(e.Body, &r); err != nil {
		return "", "", fmt.Errorf("decode release for %s: %w", repo, err)
	}
	return r.TagName, r.HTMLURL, nil
}

// FetchAll lists the user's public repositories and enriches each one with its
// language breakdown and latest release, using a bounded worker pool.
func (c *Client) FetchAll(ctx context.Context, user string) ([]Repo, error) {
	repos, err := c.ListRepos(ctx, user)
	if err != nil {
		return nil, err
	}
	workers := c.Concurrency
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := &repos[i]
				langs, err := c.Languages(ctx, user, r.Name)
				if err != nil {
					fail(err)
					continue
				}
				tag, rel, err := c.LatestRelease(ctx, user, r.Name)
				if err != nil {
					fail(err)
					continue
				}
				r.Languages, r.LatestRelease, r.ReleaseURL = langs, tag, rel
			}
		}()
	}
	for i := range repos {
		select {
		case jobs <- i:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

// ResolveToken returns GITHUB_TOKEN (or GH_TOKEN) from env, falling back to
// `gh auth token`. An empty string means unauthenticated requests.
func ResolveToken(getenv func(string) string, ghToken func() (string, error)) string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
	}
	if ghToken != nil {
		if t, err := ghToken(); err == nil {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// GHAuthToken runs `gh auth token`.
func GHAuthToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	return string(out), err
}
