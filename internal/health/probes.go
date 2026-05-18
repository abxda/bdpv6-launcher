// Package health provides cheap "is this thing reachable" checks for the
// service status badges. Two flavours: a TCP connect probe (used for raw
// HDFS RPC and Kafka broker ports) and an HTTP probe (used for the
// Elasticsearch cluster health, NameNode JMX, and Jupyter status endpoints).
// All probes have aggressive timeouts (default 2 s) because they run on the
// UI refresh path.
package health

import (
	"context"
	"net"
	"net/http"
	"time"
)

// Result is what a probe reports back to the caller. Healthy means the probe
// completed successfully within the timeout. Detail carries a short message
// suitable for surfacing in a tooltip ("connection refused", "200 OK in
// 12 ms", "no route to host").
type Result struct {
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Detail  string        `json:"detail"`
}

// TCP attempts a TCP handshake to the given host:port and returns whether it
// succeeded within the timeout. Used as the cheapest "is the service even
// listening?" check.
func TCP(addr string, timeout time.Duration) Result {
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("tcp", addr)
	lat := time.Since(start)
	if err != nil {
		return Result{Healthy: false, Latency: lat, Detail: err.Error()}
	}
	_ = conn.Close()
	return Result{Healthy: true, Latency: lat, Detail: "tcp ok"}
}

// HTTP issues a GET to the given URL and considers the response healthy if
// the status code is in the 2xx range (or 3xx, which usually means a redirect
// to a login page — still proves the server is up).
func HTTP(url string, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{Healthy: false, Detail: err.Error()}
	}
	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return Result{Healthy: false, Latency: lat, Detail: err.Error()}
	}
	defer resp.Body.Close()
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	return Result{
		Healthy: healthy,
		Latency: lat,
		Detail:  http.StatusText(resp.StatusCode),
	}
}
