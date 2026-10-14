package embedding

import (
	"io"
	"net/http"
	"time"
)

// retryableStatuses lists transient HTTP statuses worth retrying.
var retryableStatuses = map[int]bool{
	http.StatusRequestTimeout:     true,
	http.StatusTooManyRequests:    true,
	http.StatusBadGateway:         true,
	http.StatusServiceUnavailable: true,
	http.StatusGatewayTimeout:     true,
}

const (
	maxAttempts    = 3
	baseRetryDelay = 250 * time.Millisecond
)

// doWithRetry issues the request built by newReq, retrying transient failures
// (connection errors and 408/429/502/503/504) with exponential backoff. The
// caller owns the returned response body on success.
func doWithRetry(client *http.Client, newReq func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(baseRetryDelay << (attempt - 1))
		}
		req, err := newReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if !retryableStatuses[resp.StatusCode] || attempt == maxAttempts-1 {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil, lastErr
}
