// Package client provides an HTTP client for the Carrier daemon API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Agent represents an agent from the daemon API.
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Runtime     string `json:"runtime"`
	Description string `json:"description,omitempty"`
}

// AgentStatus represents the status of an agent.
type AgentStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Client talks to the Carrier daemon HTTP API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// New creates a new Client.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) do(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// Healthz checks daemon health.
func (c *Client) Healthz() error {
	_, err := c.do("GET", "/healthz", nil)
	return err
}

// ListAgents returns available agents.
func (c *Client) ListAgents() ([]Agent, error) {
	data, err := c.do("GET", "/api/agents", nil)
	if err != nil {
		return nil, err
	}
	var agents []Agent
	return agents, json.Unmarshal(data, &agents)
}

// GetStatus returns agent status.
func (c *Client) GetStatus(id string) (*AgentStatus, error) {
	data, err := c.do("GET", "/api/v1/agents/"+id+"/status", nil)
	if err != nil {
		return nil, err
	}
	var s AgentStatus
	return &s, json.Unmarshal(data, &s)
}

// Install installs an agent.
func (c *Client) Install(agentID string) error {
	_, err := c.do("POST", "/api/install", map[string]string{"agentId": agentID})
	return err
}

// Start starts an agent.
func (c *Client) Start(agentID string) error {
	_, err := c.do("POST", "/api/start", map[string]string{"agentId": agentID})
	return err
}

// Stop stops an agent.
func (c *Client) Stop(agentID string) error {
	_, err := c.do("POST", "/api/stop", map[string]string{"agentId": agentID})
	return err
}

// Logs returns agent logs.
func (c *Client) Logs(id string, tail int) (string, error) {
	data, err := c.do("GET", fmt.Sprintf("/api/v1/agents/%s/logs?tail=%d", id, tail), nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
