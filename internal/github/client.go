package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v55/github"
	"golang.org/x/oauth2"
)

type Client interface {
	GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error)
}

type RESTClient struct {
	client *github.Client
}

func New(token string) *RESTClient {
	var tc *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc = oauth2.NewClient(context.Background(), ts)
	}
	return &RESTClient{
		client: github.NewClient(tc),
	}
}

func (c *RESTClient) GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error) {
	// Split ownerRepo into owner and repo
	var owner, repo string
	parts := strings.SplitN(ownerRepo, "/", 2)
	if len(parts) != 2 {
		return RepoMeta{}, fmt.Errorf("invalid owner/repo format")
	}
	owner, repo = parts[0], parts[1]

	gr, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return RepoMeta{}, err
	}
	lic := ""
	if gr.License != nil && gr.License.GetSPDXID() != "NOASSERTION" {
		lic = gr.License.GetSPDXID()
	}
	return RepoMeta{
		FullName:    gr.GetFullName(),
		URL:         gr.GetHTMLURL(),
		Description: gr.GetDescription(),
		Stars:       gr.GetStargazersCount(),
		License:     lic,
		Topics:      gr.Topics,
		Archived:    gr.GetArchived(),
		PushedAt:    gr.GetPushedAt().Time,
	}, nil
}
