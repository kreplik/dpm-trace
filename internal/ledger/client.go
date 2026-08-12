// Package ledger is the JSON Ledger API client: update lookup, completions,
// prepare and submit-and-wait, plus bearer-token resolution.
//
// HTTP only -- the tool speaks the JSON API, so there is no gRPC or protobuf
// dependency, per the stdlib-only policy in #7.
package ledger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// API paths, mirroring the constants in cli.py.
const (
	ScanUpdatePath            = "/v2/updates/"
	UpdateByIDPath            = "/v2/updates/update-by-id"
	ActiveContractsPath       = "/v2/state/active-contracts"
	InteractiveSubmissionPath = "/v2/interactive-submission/prepare"
	SubmitAndWaitPath         = "/v2/commands/submit-and-wait"
	CompletionsPath           = "/v2/commands/completions"
)

// Retry policy for transient failures. Distinct from the ingestion --wait loop:
// this retries the same request, --wait retries because the update may not have
// been ingested yet.
const (
	RetryAttempts  = 3
	RetryBackoff   = 300 * time.Millisecond // doubled per attempt: 0.3s, 0.6s
	RequestTimeout = 20 * time.Second
)

// Client talks to one participant's JSON Ledger API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client with the default timeout.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: RequestTimeout},
	}
}

// JSON performs a request and decodes the response.
//
// retry applies to idempotent calls only: 5xx and connection-level failures are
// retried with exponential backoff, 4xx never is. Pass retry=false for
// non-idempotent calls such as submit-and-wait, where a retry could double-submit.
func (c *Client) JSON(method, url string, body map[string]any, retry bool) (*model.Object, error) {
	attempts := 1
	if retry {
		attempts = RetryAttempts
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		obj, err, transient := c.once(method, url, body)
		if err == nil {
			return obj, nil
		}
		lastErr = err
		if !transient || attempt == attempts-1 {
			return nil, err
		}
		time.Sleep(RetryBackoff * (1 << attempt))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s %s failed: exhausted retries", method, url)
	}
	return nil, lastErr
}

// once performs a single request, reporting whether the failure is retryable.
func (c *Client) once(method, url string, body map[string]any) (*model.Object, error, bool) {
	var reader io.Reader
	if body != nil {
		encoded, err := model.Encode(body)
		if err != nil {
			return nil, err, false
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err, false
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.Token))
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Connection-level: refused, timeout, DNS. Always worth one more try.
		return nil, fmt.Errorf("%s %s failed: %w", method, url, err), true
	}
	defer resp.Body.Close()

	data, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		// Keep the decoded body: for submit-and-wait a rejection carries the
		// completion data, which is all a failed submission has.
		httpErr := &HTTPError{
			Method: method, URL: url, StatusCode: resp.StatusCode, Detail: string(data),
		}
		if decoded, decodeErr := model.Decode(data); decodeErr == nil {
			httpErr.Body = decoded
		}
		return nil, httpErr, isTransientStatus(resp.StatusCode)
	}
	if readErr != nil {
		return nil, readErr, true
	}

	obj, err := model.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s %s returned invalid JSON: %w", method, url, err), false
	}
	return obj, nil, false
}

// isTransientStatus reports the 5xx codes a same-request retry can plausibly
// fix. 4xx is a client error and never retried.
func isTransientStatus(code int) bool {
	switch code {
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// JoinURL joins a base and path with exactly one separator. Ports join_url.
func JoinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// ResolveToken applies the precedence in apply_config_defaults: an explicit
// token, then a token file, then the environment.
func ResolveToken(token, tokenFile string) (string, error) {
	if token != "" {
		return token, nil
	}
	if tokenFile == "" {
		tokenFile = os.Getenv("DPM_TRACE_TOKEN_FILE")
	}
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	return os.Getenv("DPM_TRACE_TOKEN"), nil
}

// ErrNoSource is returned when neither a ledger nor a Scan endpoint is given.
var ErrNoSource = errors.New("choose a source: --scan-url BASE with target, or --ledger-url BASE with target")

// HTTPError is a non-2xx response. Body holds the decoded payload when the
// participant returned JSON, which submit-and-wait relies on.
type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Detail     string
	Body       *model.Object
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s failed with HTTP %d: %s", e.Method, e.URL, e.StatusCode, e.Detail)
}

// errorBody returns the decoded response body of an HTTP error, if any.
func errorBody(err error) *model.Object {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Body
	}
	return nil
}
