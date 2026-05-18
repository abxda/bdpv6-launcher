// Package hdfsfs is a tiny WebHDFS REST client used by the HDFS Explorer
// tab. It replaces the legacy launcher's Python hdfs client (which had a
// hardcoded user name) and does not depend on the Java HDFS client.
//
// The launcher only ever needs LISTSTATUS today; if F8/F9 grow more
// features (upload, download, mkdir), add them here behind small focused
// methods rather than a generic Do() — that keeps callers honest.
package hdfsfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Entry is one row of a directory listing.
type Entry struct {
	Name             string `json:"name"`
	Type             string `json:"type"`             // "DIRECTORY" | "FILE"
	Length           int64  `json:"length"`           // bytes
	ModificationTime int64  `json:"modificationTime"` // unix millis
	Owner            string `json:"owner"`
	Group            string `json:"group"`
	Permission       string `json:"permission"`
}

// Client wraps the NameNode HTTP endpoint and an optional user.name for
// distributions where permissions are enabled.
type Client struct {
	BaseURL string // e.g. http://127.0.0.1:9870
	User    string // optional
	HTTP    *http.Client
}

// New returns a Client with a 3s default timeout — the HDFS Explorer is on
// the UI thread so we never want to hang the launcher waiting for a slow
// NameNode.
func New(baseURL, user string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		User:    user,
		HTTP:    &http.Client{Timeout: 3 * time.Second},
	}
}

// List returns the directory children for `path` (use "/" for root). Names
// in the result are pathSuffix only (no leading slash, no parent path).
func (c *Client) List(ctx context.Context, path string) ([]Entry, error) {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	q := url.Values{}
	q.Set("op", "LISTSTATUS")
	if c.User != "" {
		q.Set("user.name", c.User)
	}
	u := c.BaseURL + "/webhdfs/v1" + escapePath(path) + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("webhdfs %s: HTTP %d — %s", path, resp.StatusCode, string(body))
	}

	var wrapper struct {
		FileStatuses struct {
			FileStatus []struct {
				PathSuffix       string `json:"pathSuffix"`
				Type             string `json:"type"`
				Length           int64  `json:"length"`
				ModificationTime int64  `json:"modificationTime"`
				Owner            string `json:"owner"`
				Group            string `json:"group"`
				Permission       string `json:"permission"`
			} `json:"FileStatus"`
		} `json:"FileStatuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("webhdfs decode: %w", err)
	}

	out := make([]Entry, 0, len(wrapper.FileStatuses.FileStatus))
	for _, s := range wrapper.FileStatuses.FileStatus {
		out = append(out, Entry{
			Name: s.PathSuffix, Type: s.Type, Length: s.Length,
			ModificationTime: s.ModificationTime, Owner: s.Owner,
			Group: s.Group, Permission: s.Permission,
		})
	}
	return out, nil
}

// escapePath does the minimum URL-encoding WebHDFS expects on a path
// component — preserves '/' but escapes spaces, accents, etc.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
