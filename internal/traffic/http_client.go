package traffic

import "net/http"

// HTTPClientLike is the minimal interface the profiler needs from the
// HTTP client. Keeps the package free of tight coupling to the
// `api` package's concrete *http.Client (and avoids an import cycle if
// `api` ever needs a callback into here).
type HTTPClientLike interface {
	Do(req *http.Request) (*http.Response, error)
}

// asStdHTTP returns a concrete *http.Client. If the supplied
// HTTPClientLike already *is* one, we reuse it; otherwise we wrap. The
// poller's only call is .Do() so the wrapping is one method.
func asStdHTTP(h HTTPClientLike) *http.Client {
	if h == nil {
		return nil
	}
	if c, ok := h.(*http.Client); ok {
		return c
	}
	return &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return h.Do(req)
		}),
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
