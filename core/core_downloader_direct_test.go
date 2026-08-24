package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// The launcher pins an exact core tag, so the asset URL is fully derivable and
// api.github.com is optional. This guards the URL shape — in particular the
// asymmetry that bit us: the git tag has a leading "v", the filename does not.
func TestDirectAssetURLShape(t *testing.T) {
	const version = "1.14.0-lx.27-rc.6"

	url, err := DirectAssetURL(version)
	if err != nil {
		t.Fatalf("DirectAssetURL(%q) failed on %s/%s: %v", version, runtime.GOOS, runtime.GOARCH, err)
	}

	wantPrefix := fmt.Sprintf("https://github.com/%s/releases/download/v%s/", coreReleaseRepo(), version)
	if !strings.HasPrefix(url, wantPrefix) {
		t.Errorf("DirectAssetURL = %q, want prefix %q", url, wantPrefix)
	}
	// The filename must NOT repeat the "v" that the tag carries.
	if strings.Contains(url, "sing-box-v") {
		t.Errorf("DirectAssetURL = %q: asset filename must not carry a 'v' prefix", url)
	}
	if !strings.HasSuffix(url, SingboxAssetSuffix()) {
		t.Errorf("DirectAssetURL = %q, want suffix %q", url, SingboxAssetSuffix())
	}
}

// A rate-limited API must not block the download: the pinned version is enough
// to build the URL ourselves.
//
// Exercises the fallback builder directly rather than getReleaseInfo, which
// hard-codes api.github.com. Going through it made the test depend on the
// network in the worst possible way: it passed only while GitHub was
// unreachable or rate-limited (locally) and failed on a CI runner that GitHub
// answers normally — the fallback never fires, so there is nothing to assert.
func TestGetReleaseInfoFallsBackWhenAPIFails(t *testing.T) {
	const version = "1.14.0-lx.27-rc.6"

	ac := &AppController{}
	release, err := buildDirectReleaseInfo(version)
	if err != nil {
		t.Fatalf("buildDirectReleaseInfo must synthesise a release, got error: %v", err)
	}
	if len(release.Assets) != 1 {
		t.Fatalf("fallback release has %d assets, want exactly the one for this platform", len(release.Assets))
	}
	if release.TagName != "v"+version {
		t.Errorf("TagName = %q, want %q", release.TagName, "v"+version)
	}
	// findPlatformAsset must accept the synthesised asset.
	if _, err := ac.findPlatformAsset(release.Assets); err != nil {
		t.Errorf("findPlatformAsset rejected the synthesised asset: %v", err)
	}
}

// GitHub answers an exhausted quota with 403 on a public endpoint. Treating that
// as a generic failure is what produced the useless "see the log" dialog.
func TestRateLimitIsRecognised(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		remaining string
		want      bool
	}{
		{"403 with exhausted quota", http.StatusForbidden, "0", true},
		{"429 with exhausted quota", http.StatusTooManyRequests, "0", true},
		{"403 with quota left is not a rate limit", http.StatusForbidden, "42", false},
		{"404 is not a rate limit", http.StatusNotFound, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.remaining != "" {
					w.Header().Set("X-RateLimit-Remaining", tc.remaining)
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			ac := &AppController{}
			_, err := ac.fetchReleaseInfo(context.Background(), srv.URL)
			if got := errors.Is(err, ErrGitHubRateLimited); got != tc.want {
				t.Errorf("errors.Is(err, ErrGitHubRateLimited) = %v, want %v (err=%v)", got, tc.want, err)
			}
		})
	}
}

// ghproxy.com answers 200 with a ~1.8 KB HTML landing page instead of the file.
// Without this guard the page is saved as "the archive" and only fails later,
// during extraction, as a misleading "corrupted archive" error.
func TestHTMLInterstitialIsRejected(t *testing.T) {
	t.Run("declared as text/html", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>GitHub Proxy</body></html>"))
		}))
		defer srv.Close()

		ac := &AppController{}
		dest := t.TempDir() + "/asset.tar.gz"
		ch := make(chan DownloadProgress, 64)
		go func() {
			for range ch {
			}
		}()
		defer close(ch)

		if err := ac.downloadFileFromURL(context.Background(), srv.URL, dest, ch); err == nil {
			t.Fatal("an HTML page served as the asset must be rejected, got success")
		}
	})

	// Some proxies serve HTML without an HTML Content-Type, so the header check
	// alone is not enough — the body must be sniffed too.
	t.Run("mislabelled as octet-stream", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>not an archive</body></html>"))
		}))
		defer srv.Close()

		ac := &AppController{}
		dest := t.TempDir() + "/asset.tar.gz"
		ch := make(chan DownloadProgress, 64)
		go func() {
			for range ch {
			}
		}()
		defer close(ch)

		if err := ac.downloadFileFromURL(context.Background(), srv.URL, dest, ch); err == nil {
			t.Fatal("an HTML body must be rejected even when mislabelled, got success")
		}
	})
}

// The mirror list must not regress back to a proxy that serves HTML interstitials.
func TestGhproxyIsNotAMirror(t *testing.T) {
	for _, m := range GitHubDownloadMirrors {
		if strings.Contains(m, "ghproxy.com") {
			t.Errorf("ghproxy.com is back in GitHubDownloadMirrors (%q): it serves an HTML page instead of the file", m)
		}
	}
	if len(GitHubDownloadMirrors) == 0 {
		t.Error("GitHubDownloadMirrors is empty: the CDN would have no fallback at all")
	}
}
