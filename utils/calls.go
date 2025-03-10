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

// POST sends a post request with body and apiKey in header to url and unmarshals the response to response.
func POST[T any](ctx context.Context, url string, apiKey APIKey, body []byte, response *T) error {
	client := &http.Client{Timeout: timeout}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	if len(apiKey.name) > 0 {
		request.Header.Set(apiKey.name, apiKey.key)
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request responded with code %d", resp.StatusCode)
	}

	respLimited := &io.LimitedReader{R: resp.Body, N: maxRespSize}
	defer resp.Body.Close()

	decoder := json.NewDecoder(respLimited)
	decoder.DisallowUnknownFields()

	err = decoder.Decode(response)
	if err != nil {
		return err
	}

	return nil
}

type ExecuteStatus[T any] struct {
	Success bool
	Err     error
	Value   T
}

func ExecuteWithRetry[T any](ctx context.Context, f func() (T, error), maxAttempts int, delay time.Duration) ExecuteStatus[T] {
	ticker := time.NewTicker(delay)
	var result ExecuteStatus[T]

	var err error
	var r T

	for j := 0; j < maxAttempts; j++ {
		if err = ctx.Err(); err != nil {
			result.Err = fmt.Errorf("context canceled mid retry: %v", err)
			return result
		}

		r, err = f()

		if err == nil {
			result.Success = true
			result.Value = r
			return result
		}

		<-ticker.C
	}

	result.Err = fmt.Errorf("max retries reached: %v", err)

	return result
}
