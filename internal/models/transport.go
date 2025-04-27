package models

import "net/http"

type AuthTransport struct {
	Token     string
	Transport http.RoundTripper
}

func (a *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+a.Token)
	return a.Transport.RoundTrip(req)
}
