package embedding

import (
	"errors"
	"fmt"
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

// TransientError marks a provider failure that is likely to succeed later
// (connection error or 408/429/502/503/504). The processor re-queues instead of
// failing the document so a short outage does not destroy queued work.
type TransientError struct{ Err error }

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// IsTransient reports whether err represents a retryable provider failure.
func IsTransient(err error) bool {
	var target *TransientError
	return errors.As(err, &target)
}

// doWithRetry issues the request built by newReq, retrying transient failures
// (connection errors and 408/429/502/503/504) with exponential backoff. The
// caller owns the returned response body on success. Exhausted transient
// failures return a *TransientError.
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
		if !retryableStatuses[resp.StatusCode] {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if attempt == maxAttempts-1 {
			return nil, &TransientError{fmt.Errorf("provider returned HTTP %d", resp.StatusCode)}
		}
	}
	return nil, &TransientError{lastErr}
}
