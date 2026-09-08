// Package testutil provides shared helpers for tests across packages.
package testutil

import "net/http"

// GetWithUA performs a GET request with the given User-Agent.
// An empty ua sends no User-Agent header.
func GetWithUA(url, ua string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	return http.DefaultClient.Do(req)
}
