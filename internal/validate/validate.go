// Package validate checks generated Markdown before it is written.
package validate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adeoluo/profile-forge/internal/config"
	"github.com/adeoluo/profile-forge/internal/github"
)

// Output runs every offline check on generated Markdown: forbidden dashes and
// mermaid syntax.
func Output(md string) error {
	var errs []error
	if hits := config.FindDashes(md); len(hits) > 0 {
		parts := make([]string, len(hits))
		for i, h := range hits {
			parts[i] = h.String()
		}
		errs = append(errs, fmt.Errorf("generated output contains em or en dashes: %s", strings.Join(parts, "; ")))
	}
	if err := Mermaid(md); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Coverage reports public, non-fork repositories that the config does not
// place in any category and does not explicitly ignore.
func Coverage(c *config.Config, repos []github.Repo) []string {
	known := map[string]bool{}
	for _, r := range c.Repos() {
		known[r] = true
	}
	for _, r := range c.Ignore {
		known[r] = true
	}
	var out []string
	for _, r := range repos {
		if !r.Fork && !r.Archived && !known[r.Name] && c.CategoryOf(r.Name) == nil {
			out = append(out, r.Name)
		}
	}
	sort.Strings(out)
	return out
}

var (
	mdImage  = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)
	htmlImg  = regexp.MustCompile(`<img[^>]*\ssrc="(https?://[^"]+)"`)
	fenceTag = regexp.MustCompile("^```")
)

// ImageURLs returns the distinct image URLs in Markdown, in first-seen order.
func ImageURLs(md string) []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range []*regexp.Regexp{mdImage, htmlImg} {
		for _, m := range re.FindAllStringSubmatch(md, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// LinkResult is the outcome of checking one URL.
type LinkResult struct {
	URL    string
	Status int
	Err    error
}

// CheckLinks requests every URL and returns an error listing those that do
// not answer HTTP 200. Requests run with bounded concurrency.
func CheckLinks(ctx context.Context, client *http.Client, urls []string, concurrency int) ([]LinkResult, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]LinkResult, len(urls))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = LinkResult{URL: u}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				results[i].Err = err
				return
			}
			req.Header.Set("User-Agent", "profile-forge link check")
			resp, err := client.Do(req)
			if err != nil {
				results[i].Err = err
				return
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			results[i].Status = resp.StatusCode
		}(i, u)
	}
	wg.Wait()
	var bad []string
	for _, r := range results {
		switch {
		case r.Err != nil:
			bad = append(bad, fmt.Sprintf("%s (%v)", r.URL, r.Err))
		case r.Status != http.StatusOK:
			bad = append(bad, fmt.Sprintf("%s (HTTP %d)", r.URL, r.Status))
		}
	}
	if len(bad) > 0 {
		return results, fmt.Errorf("%d of %d image URLs failed: %s", len(bad), len(urls), strings.Join(bad, "; "))
	}
	return results, nil
}
