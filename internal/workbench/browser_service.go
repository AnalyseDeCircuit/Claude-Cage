package workbench

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/sylvinus/agent-vm/internal/browser"
)

var ErrBrowserUnavailable = errors.New("环境尚未就绪")

func (s *Service) browserClient() *browser.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserControl == nil {
		s.browserControl = browser.New("http://127.0.0.1:18500")
	}
	return s.browserControl
}

// BrowserRequestError distinguishes guest connection failures from local
// authorization and validation errors without assigning an HTTP status.
type BrowserRequestError struct{ Err error }

func (e *BrowserRequestError) Error() string { return e.Err.Error() }
func (e *BrowserRequestError) Unwrap() error { return e.Err }

func (s *Service) browserToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready {
		return "", ErrBrowserUnavailable
	}
	return s.credentials.Token, nil
}

func (s *Service) BrowserTabs(ctx context.Context) ([]byte, error) {
	return s.browserOperation(ctx, "tabs", browser.Input{}, true)
}

func (s *Service) BrowserCommand(ctx context.Context, action string, input browser.Input) ([]byte, error) {
	return s.browserOperation(ctx, action, input, false)
}

func (s *Service) browserOperation(ctx context.Context, action string, input browser.Input, readTabs bool) ([]byte, error) {
	ctx, done, err := s.beginActivity(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	if !browser.Supports(action) {
		return nil, browser.ErrUnknownAction
	}
	token, err := s.browserToken()
	if err != nil {
		return nil, err
	}
	if action == "new" && input.URL == "" {
		s.mu.Lock()
		input.URL = s.preferences.HomeURL()
		s.mu.Unlock()
	}
	if action == "editor" || action == "editor-restart" {
		if !s.op.TryLock() {
			return nil, ErrOperationBusy
		}
		defer s.op.Unlock()
		input.Workspace, err = s.projectWorkspace(input.Workspace)
		if err != nil {
			return nil, err
		}
		if err := s.projectManager().EnsureShare(ctx, input.Workspace); err != nil {
			return nil, err
		}
	}
	var data []byte
	if readTabs {
		data, err = s.browserClient().Tabs(ctx, token)
	} else {
		data, err = s.browserClient().Command(ctx, token, action, input)
	}
	if err != nil && !errors.Is(err, browser.ErrInvalidText) {
		return nil, &BrowserRequestError{Err: err}
	}
	return data, err
}

// SubscribeBrowser holds one service activity until the caller closes its
// stream or its context ends. It does not own the guest's tabs or browser.
func (s *Service) SubscribeBrowser(ctx context.Context) (io.ReadCloser, error) {
	ctx, done, err := s.beginActivity(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.browserToken()
	if err != nil {
		done()
		return nil, err
	}
	stream, err := s.browserClient().Subscribe(ctx, token)
	if err != nil {
		done()
		return nil, &BrowserRequestError{Err: err}
	}
	var closeErr error
	finish := sync.OnceFunc(func() { closeErr = stream.Close(); done() })
	stop := context.AfterFunc(ctx, finish)
	return &browserSubscription{Reader: stream, close: func() error { stop(); finish(); return closeErr }}, nil
}

type browserSubscription struct {
	io.Reader
	close func() error
}

func (s *browserSubscription) Close() error { return s.close() }
