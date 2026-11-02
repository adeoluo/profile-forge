package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrCacheMiss is returned when an entry is absent (or stale and not allowed).
var ErrCacheMiss = errors.New("cache miss")

// Entry is one cached HTTP response.
type Entry struct {
	URL       string    `json:"url"`
	Status    int       `json:"status"`
	Link      string    `json:"link,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	Body      []byte    `json:"body"`
}

// Cache stores API responses on disk, one JSON file per URL.
type Cache struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

// NewCache returns a cache rooted at dir with the given time-to-live.
func NewCache(dir string, ttl time.Duration) *Cache {
	return &Cache{Dir: dir, TTL: ttl, Now: time.Now}
}

// DefaultDir returns the per-user cache directory for profile-forge.
func DefaultDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "profile-forge")
	}
	return ".profile-forge-cache"
}

func (c *Cache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:16])+".json")
}

// Get returns the cached entry for url. When allowStale is false, entries
// older than TTL are reported as misses.
func (c *Cache) Get(url string, allowStale bool) (*Entry, error) {
	data, err := os.ReadFile(c.path(url))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("corrupt cache entry for %s: %w", url, err)
	}
	if e.URL != url {
		return nil, ErrCacheMiss
	}
	if !allowStale && c.Now().Sub(e.FetchedAt) > c.TTL {
		return nil, ErrCacheMiss
	}
	return &e, nil
}

// Put stores an entry atomically (write to a temp file, then rename).
func (c *Cache) Put(e *Entry) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	final := c.path(e.URL)
	tmp, err := os.CreateTemp(c.Dir, ".entry-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), final)
}
