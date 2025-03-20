package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestToBytes32(t *testing.T) {
	tests := []struct {
		in  string
		err bool
	}{
		{
			in:  "",
			err: false,
		},
		{
			in:  " x ",
			err: false,
		},
		{
			in:  "12 \n",
			err: false,
		},
		{
			in:  "REG",
			err: false,
		},
		{
			in:  "TO_PAUSE_FOR_UPGRADE",
			err: false,
		},
		{
			in:  strings.Repeat("a", 33),
			err: true,
		},
	}

	for _, test := range tests {
		out, err := ToBytes32(test.in)

		if test.err {
			require.Error(t, err, test.in)
		} else {
			wordEnd := len(test.in)
			word := out[0:wordEnd]
			rest := out[wordEnd:]
			restExpected := make([]byte, 32-wordEnd)

			require.NoError(t, err, test.in)
			require.Equal(t, []byte(test.in), word, test.in)
			require.Equal(t, restExpected, rest, test.in)
		}
	}
}

func TestPOST(t *testing.T) {
	port := 4912

	handler := http.NewServeMux()

	type req struct {
		A int `json:"a"`
	}

	type res struct {
		B uint `json:"b"`
	}

	handler.HandleFunc("/abs", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)

		in := req{}
		err := decoder.Decode(&in)
		require.NoError(t, err)
		defer r.Body.Close()

		var b uint
		if in.A < 0 {
			b = uint(-in.A)
		} else {
			b = uint(in.A)
		}

		out := res{B: b}
		responseBytes, err := json.Marshal(out)
		if err != nil {
			w.WriteHeader(http.StatusInsufficientStorage)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		_, err = w.Write(responseBytes)
		if err != nil {
			w.WriteHeader(http.StatusInsufficientStorage)
		}
	})

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		err := server.ListenAndServe()
		require.Error(t, err)
		wg.Done()
	}()

	request := req{
		A: -10,
	}

	reqMarshaled, err := json.Marshal(request)
	require.NoError(t, err)

	url := fmt.Sprintf("http://localhost:%d/abs", port)

	response, err := post[res](context.Background(), url, NoAPIKey, reqMarshaled)
	require.NoError(t, err)

	expected := res{10}

	require.Equal(t, expected, *response)

	err = server.Shutdown(context.Background())
	require.NoError(t, err)

	wg.Wait()
}

const errorMsg = "still error"

var errRetry error = errors.New(errorMsg)

func testFunction(k int) func() (int, error) {
	return func() (int, error) {
		if k > 0 {
			k--
			return 0, errRetry
		}

		return 100, nil
	}
}

func testFunction2(d time.Duration) func() (int, error) {
	start := time.Now()

	return func() (int, error) {
		if time.Since(start) > d {
			return 100, nil
		}

		return 0, errRetry
	}
}

func TestExecuteWithRetry(t *testing.T) {
	tests := []struct {
		f        func() (int, error)
		params   RetryParams
		expected ExecuteStatus[int]
	}{
		{
			f: testFunction(3),
			params: RetryParams{
				MaxAttempts: 4,
				Delay:       0,
				Timeout:     0,
			},
			expected: ExecuteStatus[int]{
				Success: true,
				Err:     nil,
				Value:   100,
			},
		},
		{
			f: testFunction(3),
			params: RetryParams{
				MaxAttempts: 0,
				Delay:       0,
				Timeout:     0,
			},
			expected: ExecuteStatus[int]{
				Success: true,
				Err:     nil,
				Value:   100,
			},
		},
		{
			f: testFunction(3),
			params: RetryParams{
				MaxAttempts: 2,
				Delay:       0,
				Timeout:     0,
			},
			expected: ExecuteStatus[int]{
				Success: false,
				Err:     fmt.Errorf("max retries reached: %v", errRetry),
				Value:   0,
			},
		},
		{
			f: testFunction2(10 * time.Millisecond),
			params: RetryParams{
				MaxAttempts: 0,
				Delay:       3 * time.Millisecond,
				Timeout:     13 * time.Millisecond,
			},
			expected: ExecuteStatus[int]{
				Success: true,
				Err:     nil,
				Value:   100,
			},
		},
		{
			f: testFunction2(20 * time.Millisecond),
			params: RetryParams{
				MaxAttempts: 0,
				Delay:       2 * time.Millisecond,
				Timeout:     5 * time.Millisecond,
			},
			expected: ExecuteStatus[int]{
				Success: false,
				Err:     fmt.Errorf("context error mid retry: %v", context.DeadlineExceeded),
				Value:   0,
			},
		},
	}

	for _, test := range tests {
		ctx := context.Background()

		outcome := ExecuteWithRetry(ctx, test.f, test.params)

		require.Equal(t, test.expected, outcome)
	}
}
