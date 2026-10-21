package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// LatestReleaseAPI is GitHub's "latest release" endpoint for Quarro. It
// already excludes drafts and prereleases.
const LatestReleaseAPI = "https://api.github.com/repos/yazeed1s/quarro/releases/latest"

// releasesPage is opened instead of a release's own html_url if that URL is
// not on github.com, so a bad API response can never send the user elsewhere.
const releasesPage = "https://github.com/yazeed1s/quarro/releases"

const updateCheckTimeout = 10 * time.Second

// AppInfo identifies this build.
type AppInfo struct {
	Version string `json:"version"`
	Stage   string `json:"stage"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// UpdateInfo describes the newest published release when it is newer than
// this build; Available is false otherwise.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	URL       string `json:"url"`
}

type AppService struct {
	info      AppInfo
	latestURL string
	client    *http.Client
}

// NewAppService checks for updates against latestURL (LatestReleaseAPI in
// production; tests pass a local server).
func NewAppService(info AppInfo, latestURL string) *AppService {
	return &AppService{
		info:      info,
		latestURL: latestURL,
		client:    &http.Client{Timeout: updateCheckTimeout},
	}
}

// ParseBuildVersion reads info.version from build/config.yml, the single
// source every packaged asset's version is bumped from
// (scripts/bump-version.sh).
func ParseBuildVersion(config []byte) (string, error) {
	var parsed struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	if err := yaml.Unmarshal(config, &parsed); err != nil {
		return "", fmt.Errorf("parse build config: %w", err)
	}
	version := parsed.Info.Version
	if !semver.IsValid("v"+version) || semver.Canonical("v"+version) != "v"+version {
		return "", fmt.Errorf("build config info.version %q is not MAJOR.MINOR.PATCH[-prerelease]", version)
	}
	return version, nil
}

func (s *AppService) Info() AppInfo {
	return s.info
}

// CheckForUpdate asks GitHub for the latest release. It only runs in prod
// builds; preview/test builds report no update without any network call.
// "No releases yet" (404) is not an error.
func (s *AppService) CheckForUpdate(ctx context.Context) (UpdateInfo, error) {
	if s.info.Stage != "prod" {
		return UpdateInfo{}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.latestURL, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Quarro/"+s.info.Version)
	resp, err := s.client.Do(req)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return UpdateInfo{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("check for updates: GitHub returned %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return UpdateInfo{}, fmt.Errorf("check for updates: %w", err)
	}
	latest := "v" + strings.TrimPrefix(release.TagName, "v")
	if !semver.IsValid(latest) || semver.Compare(latest, "v"+s.info.Version) <= 0 {
		return UpdateInfo{}, nil
	}
	url := release.HTMLURL
	if !strings.HasPrefix(url, "https://github.com/") {
		url = releasesPage
	}
	return UpdateInfo{Available: true, Version: strings.TrimPrefix(latest, "v"), URL: url}, nil
}
