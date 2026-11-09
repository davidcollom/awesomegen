package awesomegen_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exercise curl's actual transient/permanent-error behaviour with the options
// used by every action download, without contacting GitHub.
func TestActionDownloadRetries(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is not installed")
	}
	data, err := os.ReadFile("action.yml")
	require.NoError(t, err)
	options := regexp.MustCompile(`--retry [0-9]+ --retry-max-time [0-9]+ --connect-timeout [0-9]+ --max-time [0-9]+`).FindAllString(string(data), -1)
	require.Len(t, options, 3, "release resolution, archive and checksum must all have bounded retries")
	require.NotContains(t, string(data), "--retry-delay", "a fixed delay disables exponential backoff")
	for _, option := range options {
		require.Equal(t, "--retry 10 --retry-max-time 540 --connect-timeout 15 --max-time 60", option)
	}
	for _, status := range []int{504, 404} {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if status == 404 || calls < 3 {
				w.WriteHeader(status)
				return
			}
			w.Write([]byte("complete archive"))
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		args := append([]string{"-fsS"}, strings.Fields(options[0])...)
		args = append(args, srv.URL)
		output, err := exec.CommandContext(ctx, "curl", args...).Output()
		cancel()
		srv.Close()
		if status == 504 {
			require.NoError(t, err)
			require.Equal(t, "complete archive", string(output))
			require.Equal(t, 3, calls)
		} else {
			require.Error(t, err)
			require.Equal(t, 1, calls)
		}
	}
}
