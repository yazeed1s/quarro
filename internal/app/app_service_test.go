package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
)

func TestParseBuildVersion(t *testing.T) {
	got, err := ParseBuildVersion([]byte("info:\n  version: \"1.2.3-rc1\" # comment\n"))
	if err != nil || got != "1.2.3-rc1" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{
		"info:\n  version: \"1.2\"\n",
		"info:\n  version: \"v1.2.3\"\n",
		"info: {}\n",
		"info: [\n",
	} {
		if v, err := ParseBuildVersion([]byte(bad)); err == nil {
			t.Errorf("ParseBuildVersion(%q) = %q, want error", bad, v)
		}
	}
}

// The packaged assets carry their own copy of the version. They must match
// build/config.yml, or the macOS About box, Windows file properties, and
// Linux package metadata would disagree with what the app reports.
func TestBuildAssetsMatchConfigVersion(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile("../../" + path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	version, err := ParseBuildVersion([]byte(read("build/config.yml")))
	if err != nil {
		t.Fatal(err)
	}
	// Numeric-only fields (bundle versions, Windows file versions) take the
	// MAJOR.MINOR.PATCH core; free-form ones take the full version.
	core := strings.TrimPrefix(semver.Canonical("v"+strings.SplitN(version, "-", 2)[0]), "v")
	for _, c := range []struct {
		path, pattern, want string
	}{
		{"build/darwin/Info.plist", `CFBundleShortVersionString</key>\s*<string>([^<]*)`, core},
		{"build/darwin/Info.plist", `CFBundleVersion</key>\s*<string>([^<]*)`, core},
		{"build/darwin/Info.dev.plist", `CFBundleShortVersionString</key>\s*<string>([^<]*)`, core},
		{"build/darwin/Info.dev.plist", `CFBundleVersion</key>\s*<string>([^<]*)`, core},
		{"build/windows/info.json", `"file_version":\s*"([^"]*)"`, core},
		{"build/windows/info.json", `"ProductVersion":\s*"([^"]*)"`, version},
		{"build/windows/wails.exe.manifest", `assemblyIdentity[^>]*version="([^"]*)"`, core},
		{"build/windows/nsis/wails_tools.nsh", `!define INFO_PRODUCTVERSION "([^"]*)"`, core},
		{"build/linux/nfpm/nfpm.yaml", `(?m)^version:\s*"([^"]*)"`, version},
	} {
		m := regexp.MustCompile(c.pattern).FindStringSubmatch(read(c.path))
		if m == nil {
			t.Errorf("%s: no match for %s", c.path, c.pattern)
			continue
		}
		if m[1] != c.want {
			t.Errorf("%s has %q, want %q; run scripts/bump-version.sh %s", c.path, m[1], c.want, version)
		}
	}
}

func releaseServer(t *testing.T, status int, tag, url string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent; GitHub rejects requests without one")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag, "html_url": url})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckForUpdate(t *testing.T) {
	const page = "https://github.com/yazeed1s/quarro/releases/tag/v0.2.0"
	for _, c := range []struct {
		name    string
		current string
		stage   string
		status  int
		tag     string
		url     string
		want    UpdateInfo
		wantErr bool
	}{
		{name: "newer", current: "0.1.0", stage: "prod", status: 200, tag: "v0.2.0", url: page,
			want: UpdateInfo{Available: true, Version: "0.2.0", URL: page}},
		{name: "tag without v", current: "0.1.0", stage: "prod", status: 200, tag: "0.2.0", url: page,
			want: UpdateInfo{Available: true, Version: "0.2.0", URL: page}},
		{name: "release beats own prerelease", current: "0.2.0-rc1", stage: "prod", status: 200, tag: "v0.2.0", url: page,
			want: UpdateInfo{Available: true, Version: "0.2.0", URL: page}},
		{name: "same", current: "0.2.0", stage: "prod", status: 200, tag: "v0.2.0", url: page},
		{name: "older", current: "0.3.0", stage: "prod", status: 200, tag: "v0.2.0", url: page},
		{name: "unparseable tag", current: "0.1.0", stage: "prod", status: 200, tag: "latest", url: page},
		{name: "foreign url falls back", current: "0.1.0", stage: "prod", status: 200, tag: "v0.2.0", url: "https://example.com/x",
			want: UpdateInfo{Available: true, Version: "0.2.0", URL: releasesPage}},
		{name: "no releases yet", current: "0.1.0", stage: "prod", status: 404},
		{name: "rate limited", current: "0.1.0", stage: "prod", status: 403, wantErr: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := releaseServer(t, c.status, c.tag, c.url)
			svc := NewAppService(AppInfo{Version: c.current, Stage: c.stage}, srv.URL)
			got, err := svc.CheckForUpdate(context.Background())
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("got %+v, %v; want %+v, err %v", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestCheckForUpdateSkipsNonProd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("non-prod build made a network request")
	}))
	defer srv.Close()
	for _, stage := range []string{"preview", "test"} {
		got, err := NewAppService(AppInfo{Version: "0.0.1", Stage: stage}, srv.URL).CheckForUpdate(context.Background())
		if err != nil || got.Available {
			t.Fatalf("%s: got %+v, %v", stage, got, err)
		}
	}
}
