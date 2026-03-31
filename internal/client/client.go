// Package client provides a typed HTTP client for communicating with ganoidd.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yashau/ganoid/internal/config"
	"github.com/yashau/ganoid/internal/event"
)

// Holder is a thread-safe, swappable client reference.
// The tray and other consumers hold a *Holder; the poller in ganoid swaps
// the inner *Client in and out as ganoidd appears or disappears.
type Holder struct {
	p atomic.Pointer[Client]
}

// Get returns the current client, or nil if ganoidd is unreachable.
func (h *Holder) Get() *Client { return h.p.Load() }

// Set replaces the current client (pass nil to mark as disconnected).
func (h *Holder) Set(c *Client) { h.p.Store(c) }

// Client talks to the ganoidd REST API.
type Client struct {
	base   string // e.g. "http://localhost:57400"
	token  string
	http   *http.Client
}

// New creates a Client pointed at the given base URL with the given auth token.
func New(port int, token string) *Client {
	return &Client{
		base:  fmt.Sprintf("http://localhost:%d", port),
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

// DashboardURL returns the URL to open in the browser (includes the auth token).
func (c *Client) DashboardURL() string {
	return fmt.Sprintf("%s/?token=%s", c.base, c.token)
}

// Status fetches the current daemon status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.get(ctx, "/api/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Profiles returns the full profile store.
func (c *Client) Profiles(ctx context.Context) (*config.Store, error) {
	var resp config.Store
	if err := c.get(ctx, "/api/profiles", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SwitchProfile streams the switch sequence for the given profile ID.
// It sends events to onEvent and calls onDone or onError when finished.
// Returns a cancel function.
func (c *Client) SwitchProfile(
	ctx context.Context,
	profileID string,
	onEvent func(event.SwitchEvent),
	onDone func(),
	onError func(error),
) context.CancelFunc {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	go func() {
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.base+"/api/profiles/"+profileID+"/switch", nil)
		if err != nil {
			onError(err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.http.Do(req)
		if err != nil {
			onError(fmt.Errorf("switch request: %w", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var e struct{ Error string `json:"error"` }
			json.NewDecoder(resp.Body).Decode(&e)
			onError(fmt.Errorf("%s", e.Error))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ev event.SwitchEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				continue
			}
			onEvent(ev)
			if ev.Done {
				if ev.Error != "" {
					onError(fmt.Errorf("%s", ev.Error))
				} else {
					onDone()
				}
				return
			}
		}
	}()
	return cancel
}

// --- low-level helpers ---

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct{ Error string `json:"error"` }
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("ganoidd %s: %s", path, e.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, body, out interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var e struct{ Error string `json:"error"` }
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("ganoidd %s: %s", path, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// StatusResponse mirrors the /api/status response shape.
type StatusResponse struct {
	ActiveProfile struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		LoginServer string `json:"login_server"`
	} `json:"active_profile"`
	Version   string `json:"version"`
	Tailscale struct {
		BackendState string `json:"backend_state"`
		PeerCount    int    `json:"peer_count"`
	} `json:"tailscale"`
}
