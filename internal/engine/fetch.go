package engine

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const defaultFetchUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// FetchBytesOptions controls a raw network fetch with optional security
// enforcement and size caps.
type FetchBytesOptions struct {
	Client    *http.Client
	Timeout   time.Duration
	Security  *SecurityPolicy
	Accept    string
	UserAgent string
}

// FetchBytes fetches raw bytes for rawURL, applying SecurityPolicy redirect,
// private-IP, and body/decompression limits when configured.
func FetchBytes(ctx context.Context, rawURL string, opts FetchBytesOptions) ([]byte, http.Header, int, error) {
	if opts.Security != nil {
		if err := opts.Security.ValidateURL(ctx, rawURL); err != nil {
			return nil, nil, 0, err
		}
	}

	client := cloneFetchClient(opts)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	if strings.TrimSpace(opts.Accept) != "" {
		req.Header.Set("Accept", opts.Accept)
	}
	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		ua = defaultFetchUserAgent
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var maxBody int64
	if opts.Security != nil {
		maxBody = opts.Security.MaxResponseBytes
	}
	body, err := limitedReadAll(resp.Body, maxBody, ErrResponseTooLarge)
	if err != nil {
		return nil, nil, resp.StatusCode, err
	}

	if encoding := strings.ToLower(resp.Header.Get("Content-Encoding")); encoding != "" {
		var maxDecomp int64
		if opts.Security != nil {
			maxDecomp = opts.Security.MaxDecompressedBytes
		}
		decoded, err := decompressBodyLimited(body, encoding, maxDecomp)
		if err != nil {
			return nil, nil, resp.StatusCode, err
		}
		body = decoded
	}

	return body, resp.Header.Clone(), resp.StatusCode, nil
}

func cloneFetchClient(opts FetchBytesOptions) *http.Client {
	var client *http.Client
	if opts.Client != nil {
		clone := *opts.Client
		client = &clone
	} else {
		base := getClient()
		clone := *base
		client = &clone
		if opts.Security != nil {
			client.Transport = &chromeTransport{security: opts.Security}
		}
	}
	if opts.Timeout > 0 {
		client.Timeout = opts.Timeout
	}
	if opts.Security != nil {
		client.CheckRedirect = opts.Security.redirectChecker(&redirectTracker{})
	}
	return client
}
