//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	recentReleases []*GitHubRelease
	recentErr      error
	latestRepo     string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.latestRepo = repo
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	githubClient := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.132",
			Name:    "v0.1.132",
		},
	}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		githubClient,
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
	require.Equal(t, githubRepo, githubClient.latestRepo)
}

func TestUpdateServicePerformUpdateRejectsDockerDeployment(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	t.Setenv("WATCHTOWER_HTTP_API_URL", "")
	t.Setenv("WATCHTOWER_HTTP_API_TOKEN", "")
	githubClient := &updateServiceGitHubClientStub{}
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		githubClient,
		"2.21.5",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.ErrorIs(t, err, ErrDockerUpdateUnsupported)
	require.Empty(t, githubClient.latestRepo)
}

func TestUpdateServiceCheckUpdateReportsDockerUpdateSupport(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	t.Setenv("WATCHTOWER_HTTP_API_URL", "http://sub2api-watchtower:8080")
	t.Setenv("WATCHTOWER_HTTP_API_TOKEN", "secret")
	t.Setenv("WATCHTOWER_HTTP_API_UPDATE", "true")

	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v2.21.5"}},
		"2.21.5",
		"release",
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.UpdateSupported)
	require.Equal(t, "docker", info.Deployment)
}

func TestUpdateServiceCheckUpdateHidesDockerUpdateWhenBridgeDisabled(t *testing.T) {
	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	t.Setenv("WATCHTOWER_HTTP_API_URL", "http://sub2api-watchtower:8080")
	t.Setenv("WATCHTOWER_HTTP_API_TOKEN", "secret")
	t.Setenv("WATCHTOWER_HTTP_API_UPDATE", "false")

	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v2.21.5"}},
		"2.21.5",
		"release",
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.False(t, info.UpdateSupported)
}

func TestUpdateServiceTriggerWatchtowerUpdate(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "2.21.5", "release")
	svc.dockerUpdateURL = server.URL
	svc.dockerToken = "secret"

	err := svc.triggerWatchtowerUpdate(context.Background())

	require.NoError(t, err)
	require.Equal(t, "/v1/update", gotPath)
	require.Equal(t, "Bearer secret", gotAuth)
}

func TestUpdateServiceTriggerWatchtowerUpdateRejectsErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "2.21.5", "release")
	svc.dockerUpdateURL = server.URL
	svc.dockerToken = "secret"

	err := svc.triggerWatchtowerUpdate(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestUpdateServicePerformUpdateDockerTriggersWatchtower(t *testing.T) {
	called := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("SUB2API_DEPLOYMENT", "docker")
	t.Setenv("WATCHTOWER_HTTP_API_URL", server.URL)
	t.Setenv("WATCHTOWER_HTTP_API_TOKEN", "secret")
	t.Setenv("WATCHTOWER_HTTP_API_UPDATE", "true")

	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "2.21.5", "release")

	require.NoError(t, svc.PerformUpdate(context.Background()))
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("watchtower update was not triggered")
	}
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148", PublishedAt: "2026-07-09T00:00:00Z"},                       // newer than current: excluded
		{TagName: "v0.1.147", PublishedAt: "2026-07-08T00:00:00Z"},                       // current: excluded
		{TagName: "v0.1.146-rc1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // prerelease: excluded
		{TagName: "v0.1.146", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.144", versions[1].Version)
	require.Equal(t, "0.1.143", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146", versions[0].Version)
	require.Equal(t, "0.1.145", versions[1].Version)
	require.Equal(t, "0.1.144", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.148"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148"},
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
		{TagName: "v0.1.145"},
		{TagName: "v0.1.144"},
		{TagName: "v0.1.143"},
		{TagName: "v0.1.142"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	for _, target := range []string{
		"",         // empty
		"0.1.147",  // current version
		"v0.1.147", // current version with prefix
		"0.1.148",  // newer than current
		"0.1.142",  // older than the 3 most recent
		"9.9.9",    // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147"},
		{TagName: "v0.1.146"},
	}
	svc := newRollbackTestService("0.1.147", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.146")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	require.Contains(t, err.Error(), "no compatible release found")
}
