package client

import (
	"context"
	"errors"
	"net/http"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/uptrace/oapi-codegen-dd/v3/pkg/runtime"

	"github.com/uptrace/terraform/internal/generated"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxRetries            = 2
	retryWaitMax          = 5 * time.Second
)

// Client wraps the generated OpenAPI client with retry and auth configuration.
type Client struct {
	// API is the oapi-codegen generated client for all spec-defined endpoints.
	API *generated.Client
}

// New creates a client with retryable HTTP transport and bearer token auth.
func New(endpoint, token string) (*Client, error) {
	rc := retryablehttp.NewClient()
	rc.HTTPClient = &http.Client{Timeout: defaultRequestTimeout}
	rc.RetryMax = maxRetries
	rc.RetryWaitMax = retryWaitMax
	rc.Logger = nil

	bearerAuth := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	apiClient, err := runtime.NewAPIClient(
		endpoint,
		runtime.WithHTTPClient(&httpDoerAdapter{client: rc.StandardClient()}),
		runtime.WithRequestEditorFn(bearerAuth),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		API: generated.NewClient(apiClient),
	}, nil
}

// IsNotFound reports whether err is an API 404 error.
func IsNotFound(err error) bool {
	return hasStatusCode(err, http.StatusNotFound)
}

// IsForbidden reports whether err is an API 403 error.
func IsForbidden(err error) bool {
	return hasStatusCode(err, http.StatusForbidden)
}

// APIErrorMessage returns the server-side message from an API error, or
// err.Error() otherwise. Needed because the generated Error.Error() is a
// stub that returns "unmapped client error" for every API response.
func APIErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if apiErr, ok := apiError(err); ok && apiErr.ErrorData.Message != "" {
		return apiErr.ErrorData.Message
	}
	return err.Error()
}

// apiError unwraps the ClientAPIError envelope to the decoded generated.Error
// value. The runtime wraps the decoded error as a value (not pointer), so we
// match on the value type.
func apiError(err error) (generated.Error, bool) {
	clientErr, ok := errors.AsType[*runtime.ClientAPIError](err)
	if !ok {
		return generated.Error{}, false
	}
	return errors.AsType[generated.Error](clientErr.Unwrap())
}

func hasStatusCode(err error, code int) bool {
	clientErr, ok := errors.AsType[*runtime.ClientAPIError](err)
	if !ok {
		return false
	}
	return clientErr.StatusCode() == code
}

// httpDoerAdapter adapts a standard *http.Client to the runtime.HttpRequestDoer
// interface which expects Do(context.Context, *http.Request).
type httpDoerAdapter struct {
	client *http.Client
}

func (a *httpDoerAdapter) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return a.client.Do(req.WithContext(ctx))
}
