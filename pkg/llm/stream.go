package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"

	"m/pkg/global"

	"github.com/aerth/mostly/stackerr"
)

const (
	Byte = 1

	KiloByte = Byte * 1000
	MegaByte = KiloByte * 1000
	GigaByte = MegaByte * 1000
	TeraByte = GigaByte * 1000

	KibiByte = Byte * 1024
	MebiByte = KibiByte * 1024
	GibiByte = MebiByte * 1024
)

// StreamPreFunc is called with the JSON request body (for debugging)
var StreamPreFunc = func(bts []byte) error { return nil }

func (c *Client) stream(ctx context.Context, method, path string, data any, fn func([]byte) error) error {
	if c == nil {
		return stackerr.Errorf("api client is nil")
	}
	if c.base == nil {
		return stackerr.Errorf("base was nil")
	}
	var buf io.Reader
	if data != nil {
		bts, err := json.Marshal(data)
		if err != nil {
			return stackerr.Errorf("json marshal request body: %w", err)
		}
		if err := StreamPreFunc(bts); err != nil {
			return stackerr.Errorf("stream prefunc: %w", err)
		}
		buf = bytes.NewBuffer(bts)
	}

	requestURL := c.base.JoinPath(path)

	var token string

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), buf)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")
	request.Header.Set("User-Agent", fmt.Sprintf("orchard/%s (%s %s) Go/%s", "0.0.0", runtime.GOARCH, runtime.GOOS, global.GetVersion()))

	if token != "" {
		request.Header.Set("Authorization", token)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	scanner := bufio.NewScanner(response.Body)
	// increase the buffer size to avoid running out of space
	scanBuf := make([]byte, 0, maxBufferSize)
	scanner.Buffer(scanBuf, maxBufferSize)
	for scanner.Scan() {

		if ctx.Err() != nil {
			return ctx.Err()
		}
		var errorResponse struct {
			Error string `json:"error,omitempty"`
			//		SigninURL string `json:"signin_url,omitempty"`
		}

		bts := scanner.Bytes()
		if err := json.Unmarshal(bts, &errorResponse); err != nil {
			if response.StatusCode >= http.StatusBadRequest {
				return StatusError{
					StatusCode:   response.StatusCode,
					Status:       response.Status,
					ErrorMessage: string(bts),
				}
			}
			slog.Error("ERROR GOT", "raw", string(bts))
			return errors.New(string(bts))
		}

		if response.StatusCode == http.StatusUnauthorized {
			return AuthorizationError{
				StatusCode: response.StatusCode,
				Status:     response.Status,
				//	SigninURL:  errorResponse.SigninURL,
			}
		} else if response.StatusCode >= http.StatusBadRequest {
			return StatusError{
				StatusCode:   response.StatusCode,
				Status:       response.Status,
				ErrorMessage: errorResponse.Error,
			}
		}

		if errorResponse.Error != "" {
			slog.Error("ERROR GOT", "raw", string(errorResponse.Error))
			return errors.New(errorResponse.Error)
		}

		if err := fn(bts); err != nil {
			slog.Error("ERROR GOT FNERR", "raw", string(err.Error()))
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return nil
}

const maxBufferSize = 8 * MegaByte
