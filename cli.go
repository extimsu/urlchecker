package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/extimsu/urlchecker/internal/probe"
	"github.com/extimsu/urlchecker/internal/target"
	"github.com/extimsu/urlchecker/metrics"
)

// SearchResult is the JSON result of one target in one-shot and metrics modes.
type SearchResult struct {
	Address      string  `json:"address"`
	Port         string  `json:"port"`
	State        string  `json:"state"`
	ResponseTime float64 `json:"response_time_seconds"`
	Group        string  `json:"group,omitempty"`
}

// GroupStatus is the health of one group.
type GroupStatus struct {
	GroupName     string   `json:"group_name"`
	IsHealthy     bool     `json:"is_healthy"`
	TotalURLs     int      `json:"total_urls"`
	HealthyURLs   int      `json:"healthy_urls"`
	UnhealthyURLs int      `json:"unhealthy_urls"`
	URLs          []string `json:"urls"`
}

// GroupResult is a group with its results, for the nested JSON output.
type GroupResult struct {
	GroupName     string         `json:"group_name"`
	IsHealthy     bool           `json:"is_healthy"`
	TotalURLs     int            `json:"total_urls"`
	HealthyURLs   int            `json:"healthy_urls"`
	UnhealthyURLs int            `json:"unhealthy_urls"`
	URLs          []SearchResult `json:"urls"`
}

// HealthCheckResult is the nested JSON output.
type HealthCheckResult struct {
	Groups        []GroupResult  `json:"groups,omitempty"`
	UngroupedURLs []SearchResult `json:"ungrouped_urls,omitempty"`
	Summary       struct {
		TotalGroups     int `json:"total_groups"`
		HealthyGroups   int `json:"healthy_groups"`
		UnhealthyGroups int `json:"unhealthy_groups"`
		TotalURLs       int `json:"total_urls"`
		HealthyURLs     int `json:"healthy_urls"`
		UnhealthyURLs   int `json:"unhealthy_urls"`
	} `json:"summary"`
}

// checker runs one-shot checks with retries.
type checker struct {
	timeout, warn, crit, retryDelay time.Duration
	retries                         int
}

// check checks t, retrying with exponential backoff, and records the metrics.
// It returns the result and the line to print.
func (c checker) check(ctx context.Context, t target.Target) (SearchResult, string) {
	start := time.Now()
	var err error
	for attempt := 0; ; attempt++ {
		if _, err = probe.Check(ctx, t, c.timeout); err == nil || attempt >= c.retries {
			break
		}
		metrics.RecordRetryAttempt(t.Addr(), t.Protocol)
		if !sleep(ctx, c.backoff(attempt)) {
			break
		}
	}
	elapsed := time.Since(start)
	metrics.RecordCheck(t.Addr(), t.Protocol, err == nil, elapsed.Seconds())
	metrics.RecordCheckDuration(t.Addr(), t.Protocol, elapsed.Seconds())

	r := SearchResult{Address: t.Host, Port: t.Port, ResponseTime: elapsed.Seconds(), Group: t.Group, State: "Success"}
	if err != nil {
		r.State = "Failed"
		return r, fmt.Sprintf("😿 [-] [%v]  %v (%.3fs) %v", t.Protocol, t.Addr(), r.ResponseTime, err)
	}
	status := "🟢"
	switch {
	case elapsed > c.crit:
		status = "🔴"
	case elapsed > c.warn:
		status = "🟡"
	}
	return r, fmt.Sprintf("%s [+] [%v]  %v (%.3fs)", status, t.Protocol, t.Addr(), r.ResponseTime)
}

// backoff is retryDelay doubled per attempt, ±10% jitter, capped at half the timeout.
func (c checker) backoff(attempt int) time.Duration {
	d := c.retryDelay << attempt
	d += time.Duration(float64(d) * 0.1 * (rand.Float64()*2 - 1))
	if d > c.timeout {
		d = c.timeout / 2
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// maxParallel bounds concurrent one-shot checks.
// ponytail: fixed bound; make it a flag if someone checks thousands of targets at once.
const maxParallel = 32

// checkAll checks every target in parallel, prints the results in input order
// and the group summary, and reports whether every target is up.
func checkAll(ctx context.Context, c checker, targets []target.Target, jsonOut bool, w io.Writer) bool {
	results := make([]SearchResult, len(targets))
	lines := make([]string, len(targets))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i], lines[i] = c.check(ctx, t)
		}()
	}
	wg.Wait()

	up := map[string]bool{}
	allUp := true
	for i, r := range results {
		up[targets[i].Key()] = r.State == "Success"
		allUp = allUp && r.State == "Success"
		if jsonOut {
			b, _ := json.Marshal(r)
			_, _ = fmt.Fprintln(w, string(b))
		} else {
			_, _ = fmt.Fprintln(w, lines[i])
		}
	}

	if groups := groupNames(targets); len(groups) > 0 {
		_, _ = fmt.Fprintln(w, "\n=== Group Health Summary ===")
		for _, g := range groups {
			gs := groupHealth(g, targets, up)
			status := "🟢"
			if !gs.IsHealthy {
				status = "🔴"
			}
			_, _ = fmt.Fprintf(w, "%s Group '%s': %d/%d URLs healthy\n", status, gs.GroupName, gs.HealthyURLs, gs.TotalURLs)
			metrics.RecordGroupHealth(gs.GroupName, gs.IsHealthy, gs.TotalURLs, gs.HealthyURLs)
		}
	}
	if jsonOut {
		writeNestedJSON(w, targets, results, up)
	}
	return allUp
}

// groupHealth counts the targets of group that are up: healthy when all are.
func groupHealth(group string, targets []target.Target, up map[string]bool) *GroupStatus {
	gs := &GroupStatus{GroupName: group, URLs: []string{}}
	for _, t := range targets {
		if t.Group != group {
			continue
		}
		gs.URLs = append(gs.URLs, t.Key())
		gs.TotalURLs++
		if up[t.Key()] {
			gs.HealthyURLs++
		}
	}
	gs.UnhealthyURLs = gs.TotalURLs - gs.HealthyURLs
	gs.IsHealthy = gs.TotalURLs > 0 && gs.UnhealthyURLs == 0
	return gs
}

// groupNames returns the non-empty groups in order of first appearance.
func groupNames(targets []target.Target) []string {
	seen := map[string]bool{}
	var groups []string
	for _, t := range targets {
		if t.Group != "" && !seen[t.Group] {
			seen[t.Group] = true
			groups = append(groups, t.Group)
		}
	}
	return groups
}

// nestedResult builds the nested JSON structure with groups and a summary.
func nestedResult(targets []target.Target, results []SearchResult, up map[string]bool) HealthCheckResult {
	var out HealthCheckResult
	byGroup := map[string][]SearchResult{}
	for i, t := range targets {
		if t.Group == "" {
			out.UngroupedURLs = append(out.UngroupedURLs, results[i])
		} else {
			byGroup[t.Group] = append(byGroup[t.Group], results[i])
		}
	}
	for _, g := range groupNames(targets) {
		gs := groupHealth(g, targets, up)
		out.Groups = append(out.Groups, GroupResult{
			GroupName: g, IsHealthy: gs.IsHealthy, TotalURLs: gs.TotalURLs,
			HealthyURLs: gs.HealthyURLs, UnhealthyURLs: gs.UnhealthyURLs, URLs: byGroup[g],
		})
		if gs.IsHealthy {
			out.Summary.HealthyGroups++
		}
	}
	out.Summary.TotalGroups = len(out.Groups)
	out.Summary.UnhealthyGroups = out.Summary.TotalGroups - out.Summary.HealthyGroups
	for _, r := range results {
		out.Summary.TotalURLs++
		if r.State == "Success" {
			out.Summary.HealthyURLs++
		}
	}
	out.Summary.UnhealthyURLs = out.Summary.TotalURLs - out.Summary.HealthyURLs
	return out
}

func writeNestedJSON(w io.Writer, targets []target.Target, results []SearchResult, up map[string]bool) {
	b, err := json.MarshalIndent(nestedResult(targets, results, up), "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error marshaling nested JSON:", err)
		return
	}
	_, _ = fmt.Fprintln(w, "\n=== Nested Group Structure ===")
	_, _ = fmt.Fprintln(w, string(b))
}
