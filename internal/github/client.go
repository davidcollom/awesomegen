package github

import (
	"context"
	"errors"
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

// ValidOwner reports whether s is a valid GitHub user or organisation name.
//
// The check lives here so that the configuration loader and the scraper
// agree on it: a configuration that names a user the scraper would refuse
// has to be rejected while the configuration is read, not when the first
// request is made.
//
// GitHub allows letters, digits and single hyphens in a user name, and
// neither a leading nor a trailing hyphen, so those are rejected as well.
func ValidOwner(s string) bool {
	if !ownerPattern.MatchString(s) {
		return false
	}
	return !strings.HasSuffix(s, "-") && !strings.Contains(s, "--")
}

// ValidRepoName reports whether s is a valid GitHub repository name.
//
// A star list slug uses the same character set as a repository name, so
// the same check applies to both.
//
// The path segments "." and ".." pass the character set but are not names:
// they address the directory they are in. They are rejected here, the way
// SplitOwnerRepo rejects them.
func ValidRepoName(s string) bool {
	if !repoPattern.MatchString(s) {
		return false
	}
	return s != "." && s != ".."
}

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
	if !ValidOwner(owner) || !ValidRepoName(repo) {
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

// IsNotFound reports whether an error says that the repository does not
// exist.
//
// The API reports a repository that was deleted, that was renamed, that
// became private or whose owner was suspended as "not found". Such an entry
// cannot be enriched, and dropping it keeps the curation of the list intact:
// the list simply does not name the repository any more.
//
// Every other failure - an expired token, a rate limit, a server error or a
// network problem - says nothing about the repository. The list is then
// unknown rather than curated, and it must not be written.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var resp *github.ErrorResponse
	if !errors.As(err, &resp) || resp.Response == nil {
		return false
	}
	return resp.Response.StatusCode == http.StatusNotFound
}
