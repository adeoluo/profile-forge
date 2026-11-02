// Package fakegh serves a minimal fake of the GitHub REST endpoints that
// profile-forge uses. It exists for tests.
package fakegh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Repo is one fake repository.
type Repo struct {
	Name        string
	Description string
	Language    string
	Languages   map[string]int64
	Topics      []string
	Stars       int
	Release     string
	Archived    bool
	Fork        bool
	Private     bool
}

// Server is a running fake API.
type Server struct {
	*httptest.Server
	User     string
	Repos    []Repo
	PageSize int
	Hits     atomic.Int64
	// AuthHeader records the last Authorization header seen.
	AuthHeader atomic.Value
}

// New starts a fake API for user with the given repositories.
func New(user string, repos []Repo) *Server {
	s := &Server{User: user, Repos: repos, PageSize: 2}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.Hits.Add(1)
	s.AuthHeader.Store(r.Header.Get("Authorization"))
	w.Header().Set("Content-Type", "application/json")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "users" && parts[2] == "repos":
		if parts[1] != s.User {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		s.listRepos(w, r)
	case len(parts) == 4 && parts[0] == "repos" && parts[3] == "languages":
		if repo := s.find(parts[2]); repo != nil {
			langs := repo.Languages
			if langs == nil {
				langs = map[string]int64{}
			}
			_ = json.NewEncoder(w).Encode(langs)
			return
		}
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	case len(parts) == 5 && parts[0] == "repos" && parts[3] == "releases" && parts[4] == "latest":
		repo := s.find(parts[2])
		if repo == nil || repo.Release == "" {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": repo.Release,
			"html_url": fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", s.User, repo.Name, repo.Release),
		})
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (s *Server) find(name string) *Repo {
	for i := range s.Repos {
		if s.Repos[i].Name == name {
			return &s.Repos[i]
		}
	}
	return nil
}

func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	start := (page - 1) * s.PageSize
	end := start + s.PageSize
	if start > len(s.Repos) {
		start = len(s.Repos)
	}
	if end > len(s.Repos) {
		end = len(s.Repos)
	}
	if end < len(s.Repos) {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(page+1))
		next := fmt.Sprintf("%s%s?%s", s.URL, r.URL.Path, q.Encode())
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next", <%s>; rel="last"`, next, next))
	}
	pushed := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	out := make([]map[string]any, 0, end-start)
	for _, repo := range s.Repos[start:end] {
		var lang any
		if repo.Language != "" {
			lang = repo.Language
		}
		var desc any
		if repo.Description != "" {
			desc = repo.Description
		}
		out = append(out, map[string]any{
			"name":             repo.Name,
			"description":      desc,
			"html_url":         fmt.Sprintf("https://github.com/%s/%s", s.User, repo.Name),
			"language":         lang,
			"topics":           repo.Topics,
			"stargazers_count": repo.Stars,
			"pushed_at":        pushed.Format(time.RFC3339),
			"archived":         repo.Archived,
			"fork":             repo.Fork,
			"private":          repo.Private,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// Sample returns a small, varied set of repositories used across tests.
func Sample() []Repo {
	return []Repo{
		{Name: "alpha", Description: "Alpha model trainer.", Language: "Python",
			Languages: map[string]int64{"Python": 9000, "Makefile": 300}, Release: "v1.2.0", Stars: 3},
		{Name: "beta", Description: "Beta retrieval engine \u2014 fast and small.", Language: "Go",
			Languages: map[string]int64{"Go": 5000, "Shell": 200}},
		{Name: "gamma", Description: "Gamma agent.", Language: "Python",
			Languages: map[string]int64{"Python": 1000}, Release: "v0.1.0"},
		{Name: "delta", Description: "", Language: "Rust",
			Languages: map[string]int64{"Rust": 700}},
		{Name: "forked", Description: "A fork.", Language: "C", Fork: true,
			Languages: map[string]int64{"C": 99999}},
		{Name: "old", Description: "Archived work.", Language: "Java", Archived: true,
			Languages: map[string]int64{"Java": 4000}},
		{Name: "hidden", Description: "Private.", Private: true},
		{Name: "stray", Description: "Not categorized.", Language: "Python",
			Languages: map[string]int64{"Python": 100}},
	}
}

// SampleConfig is a YAML config that matches Sample.
const SampleConfig = `user: tester
name: Test Person
headline: Engineer
bio: |
  Builds "careful" systems.
links:
  - label: GitHub
    url: https://github.com/tester
badges:
  - label: built with
    message: Go
    color: "00ADD8"
    logo: go
intro:
  flagships: The best ones.
footer: Generated for tests.
categories:
  - id: model
    title: Modeling
    color_name: blue
    color: "#dbeafe"
    stroke: "#1d4ed8"
    text: "#0b1b3a"
    description: Training things.
    repos: [alpha, delta]
  - id: retr
    title: Retrieval
    color_name: green
    color: "#dcfce7"
    description: Finding things.
    repos: [beta, gamma]
flagships:
  - repo: alpha
    result: Matches the reference to 1e-9.
  - repo: beta
    summary: A small search engine
    result: Recall 0.98 at k=10 | measured.
ecosystem:
  aliases:
    alpha: alpha-model
  edges:
    - {from: alpha, to: beta}
    - {from: gamma, to: beta, label: queries, style: dotted}
    - {from: delta, to: alpha, style: thick}
languages:
  top: 3
ignore: [stray]
`
