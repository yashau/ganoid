package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Profile represents a Tailscale coordination server profile.
type Profile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	LoginServer string    `json:"login_server"` // empty = official Tailscale
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    time.Time `json:"last_used"`
}

// Store holds all profiles and tracks the active one.
type Store struct {
	ActiveProfileID string    `json:"active_profile_id"`
	Profiles        []Profile `json:"profiles"`
	AuthToken       string    `json:"auth_token"`
}

// Config manages persistent profile storage.
type Config struct {
	mu   sync.RWMutex
	path string
	data Store
}

// Load reads or creates the config file at the given directory.
func Load(configDir string) (*Config, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	path := filepath.Join(configDir, "profiles.json")
	c := &Config{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		token, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("generate auth token: %w", err)
		}
		// Fresh config — seed with official Tailscale profile
		c.data = Store{
			AuthToken: token,
			ActiveProfileID: "official",
			Profiles: []Profile{
				{
					ID:          "official",
					Name:        "Tailscale Official",
					LoginServer: "",
					CreatedAt:   time.Now().UTC(),
					LastUsed:    time.Now().UTC(),
				},
			},
		}
		return c, c.save()
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &c.data); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Migrate: generate token if config predates auth.
	if c.data.AuthToken == "" {
		token, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("generate auth token: %w", err)
		}
		c.data.AuthToken = token
		if err := c.save(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// GetStore returns a copy of the current store.
func (c *Config) GetStore() Store {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// GetProfile returns a profile by ID.
func (c *Config) GetProfile(id string) (Profile, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.data.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// ActiveProfile returns the currently active profile.
func (c *Config) ActiveProfile() (Profile, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.data.Profiles {
		if p.ID == c.data.ActiveProfileID {
			return p, true
		}
	}
	return Profile{}, false
}

// AddProfile adds a new profile and persists.
func (c *Config) AddProfile(p Profile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.data.Profiles {
		if existing.ID == p.ID {
			return fmt.Errorf("profile id %q already exists", p.ID)
		}
	}
	c.data.Profiles = append(c.data.Profiles, p)
	return c.save()
}

// UpdateProfile updates an existing profile's name and login server.
func (c *Config) UpdateProfile(id, name, loginServer string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, p := range c.data.Profiles {
		if p.ID == id {
			c.data.Profiles[i].Name = name
			c.data.Profiles[i].LoginServer = loginServer
			return c.save()
		}
	}
	return fmt.Errorf("profile %q not found", id)
}

// DeleteProfile removes a profile. Returns error if it is the active profile.
func (c *Config) DeleteProfile(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data.ActiveProfileID == id {
		return fmt.Errorf("cannot delete the active profile")
	}
	profiles := c.data.Profiles[:0]
	found := false
	for _, p := range c.data.Profiles {
		if p.ID == id {
			found = true
			continue
		}
		profiles = append(profiles, p)
	}
	if !found {
		return fmt.Errorf("profile %q not found", id)
	}
	c.data.Profiles = profiles
	return c.save()
}

// SetActiveProfile updates the active profile ID and last-used timestamp.
func (c *Config) SetActiveProfile(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for i, p := range c.data.Profiles {
		if p.ID == id {
			found = true
			c.data.Profiles[i].LastUsed = time.Now().UTC()
		}
	}
	if !found {
		return fmt.Errorf("profile %q not found", id)
	}
	c.data.ActiveProfileID = id
	return c.save()
}

// AuthToken returns the shared secret used to authenticate API requests.
func (c *Config) AuthToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data.AuthToken
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (c *Config) save() error {
	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
