package httpclient

import "time"

// requestOptions holds per-request customizations applied on top of the client config.
type requestOptions struct {
	headers map[string]string
	timeout time.Duration // overrides Config.RequestTimeout for this request only
}

// Option is a functional option for a single Do call.
type Option func(*requestOptions)

// WithHeader adds or overrides a request header for this call only.
//
//	client.Do(ctx, "POST", "/payments", body, &out,
//	    httpclient.WithHeader("Idempotency-Key", key),
//	    httpclient.WithHeader("X-Tenant-ID", tenantID),
//	)
func WithHeader(key, value string) Option {
	return func(o *requestOptions) {
		if o.headers == nil {
			o.headers = make(map[string]string)
		}
		o.headers[key] = value
	}
}

// WithTimeout overrides the client-level RequestTimeout for this call only.
// Useful for endpoints known to be slower (e.g. report generation).
//
//	client.Do(ctx, "GET", "/reports/annual", nil, &out,
//	    httpclient.WithTimeout(30*time.Second),
//	)
func WithTimeout(d time.Duration) Option {
	return func(o *requestOptions) {
		o.timeout = d
	}
}

func applyOptions(opts []Option) *requestOptions {
	ro := &requestOptions{}
	for _, opt := range opts {
		opt(ro)
	}
	return ro
}
