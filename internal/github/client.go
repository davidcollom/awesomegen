package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client interface {
	GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error)
}

type RESTClient struct {
	http  *http.Client
	token string
}

func New(token string) *RESTClient {
	return &RESTClient{
		http:  &http.Client{Timeout: 10 * time.Second},
		token: token,
	}
}

func (c *RESTClient) GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+ownerRepo, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "awesomegen/1.0")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return RepoMeta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return RepoMeta{}, fmt.Errorf("github: %s", resp.Status)
	}
	var d struct {
		FullName    string `json:"full_name"`
		HTMLURL     string `json:"html_url"`
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		Archived    bool   `json:"archived"`
		PushedAt    string `json:"pushed_at"`
		License     *struct {
			SPDX string `json:"spdx_id"`
		} `json:"license"`
		Topics []string `json:"topics"`
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&d); err != nil {
		return RepoMeta{}, err
	}
	t, _ := time.Parse(time.RFC3339, d.PushedAt)
	lic := ""
	if d.License != nil && d.License.SPDX != "NOASSERTION" {
		lic = d.License.SPDX
	}
	return RepoMeta{
		FullName:    d.FullName,
		URL:         d.HTMLURL,
		Description: d.Description,
		Stars:       d.Stars,
		License:     lic,
		Topics:      d.Topics,
		Archived:    d.Archived,
		PushedAt:    t,
	}, nil
}
