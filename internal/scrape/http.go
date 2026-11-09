package scrape

import (
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
)

// A scheduled refresh can wait for GitHub to recover. Retry only transient
// failures; the library also honours Retry-After and stops on cancellation.
func newHTTPClient(log logr.Logger) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.HTTPClient.Timeout = 15 * time.Second
	client.RetryMax = 10
	client.RetryWaitMin = 2 * time.Second
	client.RetryWaitMax = 2 * time.Minute
	client.Logger = nil
	client.RequestLogHook = func(_ retryablehttp.Logger, _ *http.Request, attempt int) {
		if attempt > 0 {
			log.Info("retrying star list request", "attempt", attempt+1)
		}
	}
	return client
}
