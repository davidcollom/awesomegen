package github

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v55/github"
)

// RequestTimeout bounds a single GitHub REST API call. The default
// http.Client has no timeout at all, which lets one unresponsive connection
// hang the whole run until the CI job hits its time limit.
const RequestTimeout = 30 * time.Second

// GitHub owner and repository names are restricted to a conservative charset.
// Validating them before they reach the API also stops path traversal in
// go-github's URL construction, which does not escape path segments.
var (
	ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

type Client interface {
	GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error)
}

type RESTClient struct {
	client *github.Client
}

func New(token string) *RESTClient {
	tc := &http.Client{
		Timeout:   RequestTimeout,
		Transport: http.DefaultTransport,
		// Never forward the Authorization header to a different host or
		// to a different scheme. The header is added by the transport
		// below to every request of the chain, so a redirect to another
		// host would hand the token over, and a redirect to plain HTTP
		// would send it over the network in clear text.
		CheckRedirect: CheckRedirect,
	}
	c := github.NewClient(tc)
	if token != "" {
		// WithAuthToken adds the bearer token to the request transport, which
		// keeps the timeout and redirect policy configured above.
		c = c.WithAuthToken(token)
	}
	return &RESTClient{client: c}
}

// CheckRedirect refuses a redirect that would leak the token.
//
// The token is added to every request of a redirect chain, so a redirect to
// another host would hand it over and a redirect to plain HTTP would send
// it over the network in clear text. A redirect to the same host over the
// same scheme is followed.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return fmt.Errorf("refusing to follow redirect to a different host: %s", req.URL.Host)
	}
	if !strings.EqualFold(req.URL.Scheme, via[0].URL.Scheme) {
		return fmt.Errorf("refusing to follow redirect to a different scheme: %s", req.URL.Scheme)
	}
	return nil
}

// SplitOwnerRepo validates an "owner/repo" identifier and returns its parts.
func SplitOwnerRepo(ownerRepo string) (string, string, error) {
	owner, repo, ok := strings.Cut(ownerRepo, "/")
	if !ok || owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid owner/repo format %q", ownerRepo)
	}
	if strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("invalid owner/repo format %q", ownerRepo)
	}
	// "." and ".." pass the character allowlist but are path segments, not
	// names: go-github does not escape them, so "owner/.." would resolve to
	// the endpoint of the parent collection instead of a repository.
	if owner == "." || owner == ".." || repo == "." || repo == ".." {
		return "", "", fmt.Errorf("invalid owner/repo format %q", ownerRepo)
	}
	if !ownerPattern.MatchString(owner) || !repoPattern.MatchString(repo) {
		return "", "", fmt.Errorf("invalid owner/repo format %q", ownerRepo)
	}
	return owner, repo, nil
}

func (c *RESTClient) GetRepo(ctx context.Context, ownerRepo string) (RepoMeta, error) {
	owner, repo, err := SplitOwnerRepo(ownerRepo)
	if err != nil {
		return RepoMeta{}, err
	}

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
