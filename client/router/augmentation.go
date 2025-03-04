package router

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

func SendPost[T any](ctx context.Context, url string, apiKey apiKey, body []byte, response T) error {
	client := &http.Client{Timeout: timeout}

	request, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(apiKey.name, apiKey.key)

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
