//go:build integration

package fixture

import (
	"errors"
	"net/http"
	"net/url"
)

// MailgunTransport sends requests for the fixture endpoint to a local mock.
type MailgunTransport struct {
	Origin *url.URL
}

func (r MailgunTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "api.mailgun.net" ||
		req.URL.Path != "/v3/mg.example.test/messages" {
		return nil, errors.New("test forbids non-Mailgun endpoints")
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.Origin.Scheme, r.Origin.Host
	clone.URL = &u
	clone.Host = r.Origin.Host
	return http.DefaultTransport.RoundTrip(clone)
}
