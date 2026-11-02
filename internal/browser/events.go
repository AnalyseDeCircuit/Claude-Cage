package browser

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// Subscribe opens one cancellable guest SSE stream, preserving state payloads
// and heartbeat comments verbatim. Close releases only this subscription, not
// browser tabs or other subscribers. Reconnection is left to the consumer.
func (c *Client) Subscribe(ctx context.Context, token string) (io.ReadCloser, error) {
	transport := newTransport()
	client := &http.Client{Transport: transport, CheckRedirect: refuseRedirect}
	req, err := http.NewRequestWithContext(ctx, "GET", c.address+"/events", nil)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errors.New("Browser unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errors.New("Browser unavailable")
	}
	if response.StatusCode != 200 {
		_ = response.Body.Close()
		transport.CloseIdleConnections()
		return nil, errors.New("Browser unavailable")
	}
	return &eventStream{ReadCloser: response.Body, transport: transport}, nil
}

type eventStream struct {
	io.ReadCloser
	transport *http.Transport
}

func (s *eventStream) Close() error {
	err := s.ReadCloser.Close()
	s.transport.CloseIdleConnections()
	return err
}
