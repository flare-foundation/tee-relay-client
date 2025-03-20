package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const timeout = 5 * time.Second    // maximal duration for the server to resolve the query
const maxRespSize = 10 * (1 << 20) // 10 MB for maximal response size of the server

func NewApiKey(name, key string) APIKey {
	return APIKey{
		name: name,
		key:  key,
	}
}

type APIKey struct {
	name string
	key  string
}

var NoAPIKey = APIKey{"", ""}

// post sends a post request with body and apiKey in header to url and unmarshals the response to response.
func post[T any](ctx context.Context, url string, apiKey APIKey, body []byte) (*T, error) {
	client := &http.Client{Timeout: timeout}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")

	if len(apiKey.name) > 0 {
		request.Header.Set(apiKey.name, apiKey.key)
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request responded with code %d", resp.StatusCode)
	}

	respLimited := &io.LimitedReader{R: resp.Body, N: maxRespSize}
	defer resp.Body.Close()

	decoder := json.NewDecoder(respLimited)
	decoder.DisallowUnknownFields()

	response := new(T)

	err = decoder.Decode(response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

type ExecuteStatus[T any] struct {
	Success bool
	Err     error
	Value   T
}

type RetryParams struct {
	MaxAttempts int           // if non positive, number of attempts is not limited.
	Delay       time.Duration // minimal delay between each attempts
	Timeout     time.Duration // total maximal duration of the execution. If zero, there is no Timeout. For a single execution, the function should handle timeout.
}

func ExecuteWithRetry[T any](ctx context.Context, f func() (T, error), params RetryParams) ExecuteStatus[T] {
	var cancel context.CancelFunc

	if params.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, params.Timeout)
		defer cancel()
	}

	var ticker *time.Ticker

	if params.Delay > 0 {
		ticker = time.NewTicker(params.Delay)
	}
	var result ExecuteStatus[T]

	var err error
	var r T

	increment := 1
	attempts := params.MaxAttempts
	if params.MaxAttempts <= 0 {
		increment = 0
		attempts = 1
	}

	for j := 0; j < attempts; j += increment {
		if err = ctx.Err(); err != nil {
			result.Err = fmt.Errorf("context error mid retry: %v", err)
			return result
		}

		r, err = f()
		if err == nil {
			result.Success = true
			result.Value = r
			return result
		}

		if params.Delay > 0 {
			<-ticker.C
		}
	}

	result.Err = fmt.Errorf("max retries reached: %v", err)

	return result
}

// PostWithRetry sends a post request and retries on unsuccessful attempts according to retry parameters.
func PostWithRetry[T any](ctx context.Context, url string, apiKey APIKey, body []byte, retryParams RetryParams) (*T, error) {
	fn := func() (*T, error) {
		return post[T](ctx, url, apiKey, body)
	}

	res := ExecuteWithRetry(ctx, fn, retryParams)

	return res.Value, res.Err
}
