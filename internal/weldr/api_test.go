package weldr

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/osbuild/osbuild-composer/internal/blueprint"
	"github.com/osbuild/osbuild-composer/internal/common"
	"github.com/osbuild/osbuild-composer/internal/distro"
	"github.com/osbuild/osbuild-composer/internal/distro/test_distro"
	"github.com/osbuild/osbuild-composer/internal/distroregistry"
	"github.com/osbuild/osbuild-composer/internal/dnfjson"
	dnfjson_mock "github.com/osbuild/osbuild-composer/internal/mocks/dnfjson"
	rpmmd_mock "github.com/osbuild/osbuild-composer/internal/mocks/rpmmd"
	"github.com/osbuild/osbuild-composer/internal/ostree/mock_ostree_repo"
	"github.com/osbuild/osbuild-composer/internal/reporegistry"
	"github.com/osbuild/osbuild-composer/internal/rpmmd"
	"github.com/osbuild/osbuild-composer/internal/store"
	"github.com/osbuild/osbuild-composer/internal/target"
	"github.com/osbuild/osbuild-composer/internal/test"

	"github.com/BurntSushi/toml"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

var dnfjsonPath string

func setupDNFJSON() {
	// compile the mock-dnf-json binary to speed up tests
	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	dnfjsonPath = filepath.Join(tmpdir, "mock-dnf-json")
	cmd := exec.Command("go", "build", "-o", dnfjsonPath, "../../cmd/mock-dnf-json")
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func createWeldrAPI(tempdir string, fixtureGenerator rpmmd_mock.FixtureGenerator) (*API, *store.Store) {

	// create tempdir subdirectory for store
	dbpath, err := os.MkdirTemp(tempdir, "")
	if err != nil {
		panic(err)
	}
	fixture := fixtureGenerator(dbpath)

	rr := reporegistry.NewFromDistrosRepoConfigs(rpmmd.DistrosRepoConfigs{
		test_distro.TestDistroName: {
			test_distro.TestArchName: {
				{Name: "test-id", BaseURLs: []string{"http://example.com/test/os/x86_64"}, CheckGPG: true},
			},
		},
		test_distro.TestDistro2Name: {
			test_distro.TestArchName: {
				{Name: "test-id-2", BaseURLs: []string{"http://example.com/test-2/os/x86_64"}, CheckGPG: true},
			},
		},
	})

	distro1 := test_distro.New()
	arch, err := distro1.GetArch(test_distro.TestArchName)
	if err != nil {
		panic(err)
	}
	distro2 := test_distro.New2()

	dr, err := distroregistry.New(distro1, distro1, distro2)
	if err != nil {
		panic(err)
	}

	solver := dnfjson.NewBaseSolver("") // test solver doesn't need a cache dir
	// create tempdir subdirectory for solver response file
	dspath, err := os.MkdirTemp(tempdir, "")
	if err != nil {
		panic(err)
	}

	respfile := fixture.ResponseGenerator(dspath)
	solver.SetDNFJSONPath(dnfjsonPath, respfile)

	testApi := NewTestAPI(solver, arch, dr, rr, nil, fixture.Store, fixture.Workers, "", nil)
	return testApi, fixture.Store
}

// createWeldrAPI2 is an alternative function to createWeldrAPI, using different test architecture
// with more than a single image type
func createWeldrAPI2(tempdir string, fixtureGenerator rpmmd_mock.FixtureGenerator, distroImageTypeDenylist map[string][]string) (*API, *store.Store) {
	// create tempdir subdirectory for store
	dbpath, err := os.MkdirTemp(tempdir, "")
	if err != nil {
		panic(err)
	}
	fixture := fixtureGenerator(dbpath)

	rr := reporegistry.NewFromDistrosRepoConfigs(rpmmd.DistrosRepoConfigs{
		test_distro.TestDistroName: {
			test_distro.TestArch2Name: {
				{Name: "test-id", BaseURLs: []string{"http://example.com/test/os/x86_64"}, CheckGPG: true},
			},
		},
		test_distro.TestDistro2Name: {
			test_distro.TestArch2Name: {
				{Name: "test-id-2", BaseURLs: []string{"http://example.com/test-2/os/x86_64"}, CheckGPG: true},
			},
		},
	})

	distro1 := test_distro.New()
	arch, err := distro1.GetArch(test_distro.TestArch2Name)
	if err != nil {
		panic(err)
	}
	distro2 := test_distro.New2()

	dr, err := distroregistry.New(distro1, distro2)
	if err != nil {
		panic(err)
	}

	// create tempdir subdirectory for solver response file
	dspath, err := os.MkdirTemp(tempdir, "")
	if err != nil {
		panic(err)
	}

	solver := dnfjson.NewBaseSolver("")
	respfile := fixture.ResponseGenerator(dspath)
	solver.SetDNFJSONPath(dnfjsonPath, respfile)
	return NewTestAPI(solver, arch, dr, rr, nil, fixture.Store, fixture.Workers, "", distroImageTypeDenylist), fixture.Store
}

func TestBasic(t *testing.T) {
	var cases = []struct {
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"/api/status", http.StatusOK, `{"api":"1","db_supported":true,"db_version":"0","schema_version":"0","backend":"osbuild-composer","build":"devel","msgs":[]}`},

		{"/api/v0/projects/source/list", http.StatusOK, `{"sources":["test-id"]}`},

		{"/api/v0/projects/source/info", http.StatusNotFound, `{"errors":[{"code":404,"id":"HTTPError","msg":"Not Found"}],"status":false}`},
		{"/api/v0/projects/source/info/", http.StatusNotFound, `{"errors":[{"code":404,"id":"HTTPError","msg":"Not Found"}],"status":false}`},
		{"/api/v0/projects/source/info/foo", http.StatusOK, `{"errors":[{"id":"UnknownSource","msg":"foo is not a valid source"}],"sources":{}}`},
		{"/api/v0/projects/source/info/test-id", http.StatusOK, `{"sources":{"test-id":{"name":"test-id","type":"yum-baseurl","url":"http://example.com/test/os/x86_64","check_gpg":true,"check_ssl":true,"system":true}},"errors":[]}`},
		{"/api/v0/projects/source/info/*", http.StatusOK, `{"sources":{"test-id":{"name":"test-id","type":"yum-baseurl","url":"http://example.com/test/os/x86_64","check_gpg":true,"check_ssl":true,"system":true}},"errors":[]}`},

		{"/api/v0/blueprints/list", http.StatusOK, `{"total":1,"offset":0,"limit":1,"blueprints":["test"]}`},
		{"/api/v0/blueprints/info/", http.StatusNotFound, `{"errors":[{"code":404,"id":"HTTPError","msg":"Not Found"}],"status":false}`},
		{"/api/v0/blueprints/info/foo", http.StatusOK, `{"blueprints":[],"changes":[],"errors":[{"id":"UnknownBlueprint","msg":"foo: "}]}`},
		{"/api/v1/distros/list", http.StatusOK, `{"distros": ["test-distro", "test-distro-2"]}`},
		{"/api/v1/compose/types", http.StatusOK, `{"types": [{"enabled":true, "name":"test_ostree_type"},{"enabled":true, "name":"test_type"}]}`},
		{"/api/v1/compose/types?distro=test-distro-2", http.StatusOK, `{"types": [{"enabled":true, "name":"test_ostree_type"},{"enabled":true, "name":"test_type"}]}`},
		{"/api/v1/compose/types?distro=fedora-1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	for _, c := range cases {
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestBlueprintsNew(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[],"version":""}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages:}`, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"400 Bad Request: The browser (or proxy) sent a request that this server could not understand: unexpected EOF"}]}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`, http.StatusBadRequest, `{"status":false,"errors":[{"id":"InvalidChars","msg":"Invalid characters in API path"}]}`},
		{"POST", "/api/v0/blueprints/new", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"Missing blueprint"}]}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","distro":"test-distro","packages":[],"version":""}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"test2","description":"Test 2","distro":"test-distro-2","packages":[],"version":""}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","distro":"fedora-1","packages":[],"version":""}`, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"'fedora-1' is not a valid distribution"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestBlueprintsNewToml(t *testing.T) {
	blueprint := `
name = "test"
description = "Test"
version = "0.0.0"

[[packages]]
name = "httpd"
version = "2.4.*"`

	req := httptest.NewRequest("POST", "/api/v0/blueprints/new", bytes.NewReader([]byte(blueprint)))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusOK, r.StatusCode)
}

func TestBlueprintsEmptyToml(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v0/blueprints/new", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
}

func TestBlueprintsInvalidToml(t *testing.T) {
	blueprint := `
name = "test"
description = "Test"
version = "0.0.0"

[[packages
name = "httpd"
version = "2.4.*"`

	req := httptest.NewRequest("POST", "/api/v0/blueprints/new", bytes.NewReader([]byte(blueprint)))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
}

func TestBlueprintsWorkspaceJSON(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"POST", "/api/v0/blueprints/workspace", `{"name":"test","description":"Test","packages":[{"name":"systemd","version":"123"}],"version":"0.0.0"}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/blueprints/workspace", `{"name":"test","description":"Test","packages:}`, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"400 Bad Request: The browser (or proxy) sent a request that this server could not understand: unexpected EOF"}]}`},
		{"POST", "/api/v0/blueprints/workspace", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"Missing blueprint"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestBlueprintsWorkspaceTOML(t *testing.T) {
	blueprint := `
name = "test"
description = "Test"
version = "0.0.0"

[[packages]]
name = "httpd"
version = "2.4.*"`

	req := httptest.NewRequest("POST", "/api/v0/blueprints/workspace", bytes.NewReader([]byte(blueprint)))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusOK, r.StatusCode)
}

func TestBlueprintsWorkspaceEmptyTOML(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v0/blueprints/workspace", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
}

func TestBlueprintsWorkspaceInvalidTOML(t *testing.T) {
	blueprint := `
name = "test"
description = "Test"
version = "0.0.0"

[[packages
name = "httpd"
version = "2.4.*"`

	req := httptest.NewRequest("POST", "/api/v0/blueprints/workspace", bytes.NewReader([]byte(blueprint)))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
}

func TestBlueprintsInfo(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"GET", "/api/v0/blueprints/info/test1", ``, http.StatusOK, `{"blueprints":[{"name":"test1","description":"Test","distro":"","modules":[],"packages":[{"name":"httpd","version":"2.4.*"}],"groups":[],"version":"0.0.0"}],
		"changes":[{"name":"test1","changed":false}], "errors":[]}`},
		{"GET", "/api/v0/blueprints/info/test2", ``, http.StatusOK, `{"blueprints":[{"name":"test2","description":"Test","distro":"","modules":[],"packages":[{"name":"systemd","version":"123"}],"groups":[],"version":"0.0.0"}],
		"changes":[{"name":"test2","changed":true}], "errors":[]}`},
		{"GET", "/api/v0/blueprints/info/test3-non", ``, http.StatusOK, `{"blueprints":[],"changes":[],"errors":[{"id":"UnknownBlueprint","msg":"test3-non: "}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"test1","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"test2","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/workspace", `{"name":"test2","description":"Test","packages":[{"name":"systemd","version":"123"}],"version":"0.0.0"}`)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/test2", ``)
		test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/test1", ``)
	}
}

func TestBlueprintsInfoToml(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"test1","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)

	req := httptest.NewRequest("GET", "/api/v0/blueprints/info/test1?format=toml", nil)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got blueprint.Blueprint
	_, err := toml.NewDecoder(resp.Body).Decode(&got)
	require.NoErrorf(t, err, "error decoding toml file")

	expected := blueprint.Blueprint{
		Name:        "test1",
		Description: "Test",
		Version:     "0.0.0",
		Distro:      "",
		Packages: []blueprint.Package{
			{
				Name:    "httpd",
				Version: "2.4.*"},
		},
		Groups:  []blueprint.Group{},
		Modules: []blueprint.Package{},
	}
	require.Equalf(t, expected, got, "received unexpected blueprint")
}

func TestBlueprintsCustomizationInfoToml(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)

	// A test blueprint with all the available customizations filled in
	// Converted from TOML to JSON for the POST
	testBlueprint := `{
  "name": "example-custom-base",
  "description": "A base system with customizations",
  "version": "0.0.1",
  "distro": "test-distro",
  "packages": [
    {
      "name": "tmux",
      "version": "*"
    },
    {
      "name": "git",
      "version": "*"
    },
    {
      "name": "vim-enhanced",
      "version": "*"
    }
  ],
  "containers": [
    {
      "source": "quay.io/fedora/fedora:latest"
    }
  ],
  "customizations": {
    "hostname": "custombase",
    "kernel": {
      "append": "nosmt=force"
    },
    "timezone": {
      "timezone": "US/Eastern",
      "ntpservers": [
        "0.north-america.pool.ntp.org",
        "1.north-america.pool.ntp.org"
      ]
    },
    "locale": {
      "languages": [
        "en_US.UTF-8"
      ],
      "keyboard": "us"
    },
    "sshkey": [
      {
        "user": "root",
        "key": "A SSH KEY FOR ROOT"
      }
    ],
    "firewall": {
      "ports": [
        "22:tcp",
        "80:tcp",
        "imap:tcp",
        "53:tcp",
        "53:udp"
      ],
      "services": {
        "enabled": [
          "ftp",
          "ntp",
          "dhcp"
        ],
        "disabled": [
          "telnet"
        ]
      }
    },
    "services": {
      "enabled": [
        "sshd",
        "cockpit.socket",
        "httpd"
      ],
      "disabled": [
        "postfix",
        "telnetd"
      ]
    },
    "user": [
      {
        "name": "admin",
        "description": "Widget admin account",
        "password": "$6$CHO2$3rN8eviE2t50lmVyBYihTgVRHcaecmeCk31LeOUleVK/R/aeWVHVZDi26zAH.o0ywBKH9Tc0/wm7sW/q39uyd1",
        "home": "/srv/widget/",
        "shell": "/usr/bin/bash",
        "groups": [
          "widget",
          "users",
          "students"
        ],
        "uid": 1200
      }
    ],
    "group": [
      {
        "name": "widget"
      },
      {
        "name": "students"
      }
    ],
    "filesystem": [
      {
        "mountpoint": "/",
        "minsize": 2147483648
      }
    ],
	"openscap": {
	    "datastream": "/usr/share/xml/scap/ssg/content/ssg-rhel8-ds.xml",
		"profile_id": "xccdf_org.ssgproject.content_profile_cis"
	}
  }
}`
	resp := test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", testBlueprint)
	body, err := io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	req := httptest.NewRequest("GET", "/api/v0/blueprints/info/example-custom-base?format=toml", nil)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)

	resp = recorder.Result()
	body, err = io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var got blueprint.Blueprint
	err = toml.Unmarshal(body, &got)
	require.NoErrorf(t, err, "error decoding toml file")

	expected := blueprint.Blueprint{
		Name:        "example-custom-base",
		Description: "A base system with customizations",
		Version:     "0.0.1",
		Distro:      "test-distro",
		Packages: []blueprint.Package{
			blueprint.Package{Name: "tmux", Version: "*"},
			blueprint.Package{Name: "git", Version: "*"},
			blueprint.Package{Name: "vim-enhanced", Version: "*"},
		},
		Modules: []blueprint.Package{},
		Groups:  []blueprint.Group{},
		Containers: []blueprint.Container{
			blueprint.Container{
				Source: "quay.io/fedora/fedora:latest",
			},
		},
		Customizations: &blueprint.Customizations{
			Hostname: common.ToPtr("custombase"),
			Kernel: &blueprint.KernelCustomization{
				Append: "nosmt=force",
			},
			SSHKey: []blueprint.SSHKeyCustomization{
				blueprint.SSHKeyCustomization{User: "root", Key: "A SSH KEY FOR ROOT"},
			},
			User: []blueprint.UserCustomization{
				blueprint.UserCustomization{
					Name:        "admin",
					Description: common.ToPtr("Widget admin account"),
					Password:    common.ToPtr("$6$CHO2$3rN8eviE2t50lmVyBYihTgVRHcaecmeCk31LeOUleVK/R/aeWVHVZDi26zAH.o0ywBKH9Tc0/wm7sW/q39uyd1"),
					Home:        common.ToPtr("/srv/widget/"),
					Shell:       common.ToPtr("/usr/bin/bash"),
					Groups:      []string{"widget", "users", "students"},
					UID:         common.ToPtr(1200),
				},
			},
			Group: []blueprint.GroupCustomization{
				blueprint.GroupCustomization{
					Name: "widget",
				},
				blueprint.GroupCustomization{
					Name: "students",
				},
			},
			Timezone: &blueprint.TimezoneCustomization{
				Timezone:   common.ToPtr("US/Eastern"),
				NTPServers: []string{"0.north-america.pool.ntp.org", "1.north-america.pool.ntp.org"},
			},
			Locale: &blueprint.LocaleCustomization{
				Languages: []string{"en_US.UTF-8"},
				Keyboard:  common.ToPtr("us"),
			},
			Firewall: &blueprint.FirewallCustomization{
				Ports: []string{"22:tcp", "80:tcp", "imap:tcp", "53:tcp", "53:udp"},
				Services: &blueprint.FirewallServicesCustomization{
					Enabled:  []string{"ftp", "ntp", "dhcp"},
					Disabled: []string{"telnet"},
				},
			},
			Services: &blueprint.ServicesCustomization{
				Enabled:  []string{"sshd", "cockpit.socket", "httpd"},
				Disabled: []string{"postfix", "telnetd"},
			},
			Filesystem: []blueprint.FilesystemCustomization{
				blueprint.FilesystemCustomization{
					Mountpoint: "/",
					MinSize:    2147483648,
				},
			},
			OpenSCAP: &blueprint.OpenSCAPCustomization{
				DataStream: "/usr/share/xml/scap/ssg/content/ssg-rhel8-ds.xml",
				ProfileID:  "xccdf_org.ssgproject.content_profile_cis",
			},
		},
	}

	require.Equalf(t, expected, got, string(body))
}

func TestNonExistentBlueprintsInfoToml(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	req := httptest.NewRequest("GET", "/api/v0/blueprints/info/test3-non?format=toml", nil)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)

	resp := recorder.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSetPkgEVRA(t *testing.T) {

	// Sorted list of dependencies
	deps := []rpmmd.PackageSpec{
		{
			Name:    "dep-package1",
			Epoch:   0,
			Version: "1.33",
			Release: "2.fc30",
			Arch:    "x86_64",
		},
		{
			Name:    "dep-package2",
			Epoch:   0,
			Version: "2.9",
			Release: "1.fc30",
			Arch:    "x86_64",
		},
		{
			Name:    "dep-package3",
			Epoch:   7,
			Version: "3.0.3",
			Release: "1.fc30",
			Arch:    "x86_64",
		},
	}
	pkgs := []blueprint.Package{
		{Name: "dep-package1", Version: "*"},
		{Name: "dep-package2", Version: "*"},
	}
	// Replace globs with dependencies
	err := setPkgEVRA(deps, pkgs)
	require.NoErrorf(t, err, "setPkgEVRA failed")
	require.Equalf(t, "1.33-2.fc30.x86_64", pkgs[0].Version, "setPkgEVRA Unexpected pkg version")
	require.Equalf(t, "2.9-1.fc30.x86_64", pkgs[1].Version, "setPkgEVRA Unexpected pkg version")

	// Test that a missing package in deps returns an error
	pkgs = []blueprint.Package{
		{Name: "dep-package1", Version: "*"},
		{Name: "dep-package0", Version: "*"},
	}
	err = setPkgEVRA(deps, pkgs)
	require.EqualErrorf(t, err, "dep-package0 missing from depsolve results", "setPkgEVRA missing package failed to return error")
}

func TestBlueprintsFreeze(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var cases = []struct {
			Fixture        rpmmd_mock.FixtureGenerator
			Path           string
			ExpectedStatus int
			ExpectedJSON   string
		}{
			{rpmmd_mock.BaseFixture, "/api/v0/blueprints/freeze/test,test2", http.StatusOK, freezeTestResponse},
		}

		tempdir := t.TempDir()

		for _, c := range cases {
			api, _ := createWeldrAPI(tempdir, c.Fixture)
			test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"dep-package1","version":"*"},{"name":"dep-package3","version":"*"}], "modules":[{"name":"dep-package2","version":"*"}],"version":"0.0.0"}`)
			test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test2","description":"Test","packages":[{"name":"dep-package1","version":"*"},{"name":"dep-package3","version":"*"}], "modules":[{"name":"dep-package2","version":"*"}],"version":"0.0.0"}`)
			test.TestRoute(t, api, false, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
		}
	})

	t.Run("toml", func(t *testing.T) {
		var cases = []struct {
			Fixture        rpmmd_mock.FixtureGenerator
			Path           string
			ExpectedStatus int
			ExpectedTOML   string
		}{
			{rpmmd_mock.BaseFixture, "/api/v0/blueprints/freeze/test?format=toml", http.StatusOK, "name=\"test\"\n description=\"Test\"\n distro=\"\"\n version=\"0.0.1\"\n groups = []\n [[packages]]\n name=\"dep-package1\"\n version=\"1.33-2.fc30.x86_64\"\n [[packages]]\n name=\"dep-package3\"\n version=\"7:3.0.3-1.fc30.x86_64\"\n [[modules]]\n name=\"dep-package2\"\n version=\"2.9-1.fc30.x86_64\""},
			{rpmmd_mock.BaseFixture, "/api/v0/blueprints/freeze/missing?format=toml", http.StatusOK, ""},
		}

		tempdir := t.TempDir()

		for _, c := range cases {
			api, _ := createWeldrAPI(tempdir, c.Fixture)
			test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"dep-package1","version":"*"},{"name":"dep-package3","version":"*"}], "modules":[{"name":"dep-package2","version":"*"}],"version":"0.0.0"}`)
			test.TestTOMLRoute(t, api, false, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedTOML)
		}
	})

	t.Run("toml-multiple", func(t *testing.T) {
		var cases = []struct {
			Fixture        rpmmd_mock.FixtureGenerator
			Path           string
			ExpectedStatus int
			ExpectedJSON   string
		}{
			{rpmmd_mock.BaseFixture, "/api/v0/blueprints/freeze/test,test2?format=toml", http.StatusBadRequest, "{\"status\":false,\"errors\":[{\"id\":\"HTTPError\",\"msg\":\"toml format only supported when requesting one blueprint\"}]}"},
		}

		tempdir := t.TempDir()

		for _, c := range cases {
			api, _ := createWeldrAPI(tempdir, c.Fixture)
			test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"dep-package1","version":"*"},{"name":"dep-package3","version":"*"}], "modules":[{"name":"dep-package2","version":"*"}],"version":"0.0.0"}`)
			test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test2","description":"Test","packages":[{"name":"dep-package1","version":"*"},{"name":"dep-package3","version":"*"}], "modules":[{"name":"dep-package2","version":"*"}],"version":"0.0.0"}`)
			test.TestRoute(t, api, false, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
		}
	})
}

func TestBlueprintsDiff(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"GET", "/api/v0/blueprints/diff/test/NEWEST/WORKSPACE", ``, http.StatusOK, `{"diff":[{"new":{"Package":{"name":"systemd","version":"123"}},"old":null},{"new":null,"old":{"Package":{"name":"httpd","version":"2.4.*"}}}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/workspace", `{"name":"test","description":"Test","packages":[{"name":"systemd","version":"123"}],"version":"0.0.0"}`)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/test", ``)
	}
}

func TestBlueprintsDelete(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"DELETE", "/api/v0/blueprints/delete/test", ``, http.StatusOK, `{"status":true}`},
		{"DELETE", "/api/v0/blueprints/delete/test3-non", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"BlueprintsError","msg":"Unknown blueprint: test3-non"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/test", ``)
	}
}

func TestBlueprintsChanges(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	rand.Seed(time.Now().UnixNano())
	// math/rand is good enough in this case
	/* #nosec G404 */
	id := strconv.Itoa(rand.Int())
	ignoreFields := []string{"commit", "timestamp"}

	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/failing"+id, ``, http.StatusOK, `{"blueprints":[],"errors":[{"id":"UnknownBlueprint","msg":"failing`+id+`"}],"limit":20,"offset":0}`, ignoreFields...)
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/"+id, ``, http.StatusOK, `{"blueprints":[{"changes":[{"commit":"","message":"Recipe `+id+`, version 0.0.0 saved.","revision":null,"timestamp":""}],"name":"`+id+`","total":1}],"errors":[],"limit":20,"offset":0}`, ignoreFields...)
	test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/"+id, ``)
	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.0"}`)
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/"+id, ``, http.StatusOK, `{"blueprints":[{"changes":[{"commit":"","message":"Recipe `+id+`, version 0.0.0 saved.","revision":null,"timestamp":""},{"commit":"","message":"Recipe `+id+`, version 0.0.0 saved.","revision":null,"timestamp":""}],"name":"`+id+`","total":2}],"errors":[],"limit":20,"offset":0}`, ignoreFields...)
	test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/"+id, ``)

	// Test with an empty Version
	/* #nosec G404 */
	id = strconv.Itoa(rand.Int())
	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":""}`)
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/"+id, ``, http.StatusOK, `{"blueprints":[{"changes":[{"commit":"","message":"Recipe `+id+`, version 0.0.0 saved.","revision":null,"timestamp":""}],"name":"`+id+`","total":1}],"errors":[],"limit":20,"offset":0}`, ignoreFields...)
	test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/"+id, ``)
}

// TestBlueprintChange tests getting a single blueprint commit
func TestBlueprintChange(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	rand.Seed(time.Now().UnixNano())
	// math/rand is good enough in this case
	/* #nosec G404 */
	id := strconv.Itoa(rand.Int())

	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.1"}`)
	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.2"}`)

	resp := test.SendHTTP(api, true, "GET", "/api/v0/blueprints/changes/"+id, ``)
	body, err := io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var changes BlueprintsChangesV0
	err = json.Unmarshal(body, &changes)
	require.Nil(t, err)
	require.Equal(t, 1, len(changes.BlueprintsChanges))
	require.Equal(t, 2, len(changes.BlueprintsChanges[0].Changes))
	commit := changes.BlueprintsChanges[0].Changes[1].Commit

	// Get the blueprint's oldest commit
	route := fmt.Sprintf("/api/v1/blueprints/change/%s/%s", id, commit)
	resp = test.SendHTTP(api, true, "GET", route, ``)
	body, err = io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var bp blueprint.Blueprint
	err = json.Unmarshal(body, &bp)
	require.Nil(t, err)
	require.Equal(t, "0.0.1", bp.Version)
}

func TestBlueprintsDepsolve(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, http.StatusOK, depsolveTestResponse},
		{rpmmd_mock.NonExistingPackage, http.StatusOK, depsolvePackageNotExistError},
		{rpmmd_mock.BadDepsolve, http.StatusOK, depsolveBadError},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.SendHTTP(api, false, "POST", "/api/v0/blueprints/new", `{"name":"test","description":"Test","packages":[{"name":"dep-package1","version":"*"}],"modules":[{"name":"dep-package3","version":"*"}],"version":"0.0.0"}`)
		test.TestRoute(t, api, false, "GET", "/api/v0/blueprints/depsolve/test", ``, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, false, "DELETE", "/api/v0/blueprints/delete/test", ``)
	}
}

// TestOldBlueprintsUndo run tests with blueprint changes after a service restart
// Old blueprints are not saved, after a restart the changes are listed, but cannot be recalled
func TestOldBlueprintsUndo(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.OldChangesFixture)
	rand.Seed(time.Now().UnixNano())
	// math/rand is good enough in this case
	/* #nosec G404 */
	ignoreFields := []string{"commit", "timestamp"}

	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/test-old-changes", ``, http.StatusOK, oldBlueprintsUndoResponse, ignoreFields...)

	resp := test.SendHTTP(api, true, "GET", "/api/v0/blueprints/changes/test-old-changes", ``)
	body, err := io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var changes BlueprintsChangesV0
	err = json.Unmarshal(body, &changes)
	require.Nil(t, err)
	require.Equal(t, 1, len(changes.BlueprintsChanges))
	require.Equal(t, 3, len(changes.BlueprintsChanges[0].Changes))
	commit := changes.BlueprintsChanges[0].Changes[2].Commit

	// Undo a known commit, that is old
	test.TestRoute(t, api, true, "POST", "/api/v0/blueprints/undo/test-old-changes/"+commit, ``, http.StatusBadRequest, `{"errors":[{"id":"BlueprintsError", "msg":"no blueprint found for commit `+commit+`"}], "status":false}`)

	// Check to make sure the undo is not present (can't undo something not there)
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/test-old-changes", ``, http.StatusOK, oldBlueprintsUndoResponse, ignoreFields...)

	// Check to make sure it didn't create an empty blueprint
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/list", ``, http.StatusOK, `{"total":1,"offset":0,"limit":1,"blueprints":["test-old-changes"]}`)

	test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/test-old-changes", ``)
}

// TestNewBlueprintsUndo run tests with blueprint changes without a service restart
func TestNewBlueprintsUndo(t *testing.T) {
	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	rand.Seed(time.Now().UnixNano())
	// math/rand is good enough in this case
	/* #nosec G404 */
	id := strconv.Itoa(rand.Int())
	ignoreFields := []string{"commit", "timestamp"}

	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}],"version":"0.0.1"}`)
	test.SendHTTP(api, true, "POST", "/api/v0/blueprints/new", `{"name":"`+id+`","description":"Test","packages":[{"name":"httpd","version":"2.4.*"}, {"name": "tmux", "version":"*"}],"version":"0.1.0"}`)

	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/"+id, ``, http.StatusOK, `{"blueprints":[{"changes":[{"commit":"","message":"Recipe `+id+`, version 0.1.0 saved.","revision":null,"timestamp":""},{"commit":"","message":"Recipe `+id+`, version 0.0.1 saved.","revision":null,"timestamp":""}],"name":"`+id+`","total":2}],"errors":[],"limit":20,"offset":0}`, ignoreFields...)

	resp := test.SendHTTP(api, true, "GET", "/api/v0/blueprints/changes/"+id, ``)
	body, err := io.ReadAll(resp.Body)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var changes BlueprintsChangesV0
	err = json.Unmarshal(body, &changes)
	require.Nil(t, err)
	require.Equal(t, 1, len(changes.BlueprintsChanges))
	require.Equal(t, 2, len(changes.BlueprintsChanges[0].Changes))
	commit := changes.BlueprintsChanges[0].Changes[1].Commit

	// Undo an unknown commit
	test.TestRoute(t, api, true, "POST", "/api/v0/blueprints/undo/"+id+"/d7e5fa641aad45300242a0f273827576e32bfc03", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownCommit","msg":"Unknown commit"}]}`)

	// Undo a known commit
	test.TestRoute(t, api, true, "POST", "/api/v0/blueprints/undo/"+id+"/"+commit, ``, http.StatusOK, `{"status":true}`)

	// Check to make sure the undo is present
	test.TestRoute(t, api, true, "GET", "/api/v0/blueprints/changes/"+id, ``, http.StatusOK, `{"blueprints":[{"changes":[{"commit":"","message":"`+id+`.toml reverted to commit `+commit+`","revision":null,"timestamp":""},{"commit":"","message":"Recipe `+id+`, version 0.1.0 saved.","revision":null,"timestamp":""},{"commit":"","message":"Recipe `+id+`, version 0.0.1 saved.","revision":null,"timestamp":""}],"name":"`+id+`","total":3}],"errors":[],"limit":20,"offset":0}`, ignoreFields...)

	test.SendHTTP(api, true, "DELETE", "/api/v0/blueprints/delete/"+id, ``)
}

func TestCompose(t *testing.T) {
	arch, err := test_distro.New().GetArch(test_distro.TestArchName)
	require.NoError(t, err)
	imgType, err := arch.GetImageType(test_distro.TestImageTypeName)
	require.NoError(t, err)
	manifest, _, err := imgType.Manifest(nil, distro.ImageOptions{}, nil, nil, nil, 0)
	require.NoError(t, err)

	ostreeImgType, err := arch.GetImageType(test_distro.TestImageTypeOSTree)
	require.NoError(t, err)
	ostreeManifest, _, err := ostreeImgType.Manifest(nil, distro.ImageOptions{}, nil, nil, nil, 0)
	require.NoError(t, err)

	expectedComposeLocal := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   imgType,
			Manifest:    manifest,
			Targets: []*target.Target{
				{
					ImageName: imgType.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType.Filename(),
						ExportName:     imgType.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}
	expectedComposeLocalAndAws := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   imgType,
			Manifest:    manifest,
			Targets: []*target.Target{
				{
					ImageName: imgType.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType.Filename(),
						ExportName:     imgType.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
				{
					Name:      target.TargetNameAWS,
					Status:    common.IBWaiting,
					ImageName: "test_upload",
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType.Filename(),
						ExportName:     imgType.Exports()[0],
					},
					Options: &target.AWSTargetOptions{
						Region:          "frankfurt",
						AccessKeyID:     "accesskey",
						SecretAccessKey: "secretkey",
						Bucket:          "clay",
						Key:             "imagekey",
					},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}
	expectedComposeOSTree := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   ostreeImgType,
			Manifest:    ostreeManifest,
			Targets: []*target.Target{
				{
					ImageName: ostreeImgType.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: ostreeImgType.Filename(),
						ExportName:     ostreeImgType.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}

	// For 2nd distribution
	arch2, err := test_distro.New2().GetArch(test_distro.TestArchName)
	require.NoError(t, err)
	imgType2, err := arch2.GetImageType(test_distro.TestImageTypeName)
	require.NoError(t, err)
	manifest2, _, err := imgType.Manifest(nil, distro.ImageOptions{}, nil, nil, nil, 0)
	require.NoError(t, err)

	expectedComposeGoodDistro := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test-distro-2",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
			Distro:         "test-distro-2",
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   imgType2,
			Manifest:    manifest2,
			Targets: []*target.Target{
				{
					ImageName: imgType2.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType2.Filename(),
						ExportName:     imgType2.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}

	// create two ostree repos, one to serve the default test_distro ref (for fallback tests) and one to serve a custom ref
	ostreeRepoDefault := mock_ostree_repo.Setup(test_distro.New().OSTreeRef())
	defer ostreeRepoDefault.TearDown()
	ostreeRepoOther := mock_ostree_repo.Setup("some/other/ref")
	defer ostreeRepoOther.TearDown()

	var cases = []struct {
		External        bool
		Method          string
		Path            string
		Body            string
		ExpectedStatus  int
		ExpectedJSON    string
		ExpectedCompose *store.Compose
		IgnoreFields    []string
	}{
		{
			true,
			"POST",
			"/api/v0/compose",
			fmt.Sprintf(`{"blueprint_name": "http-server","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			http.StatusBadRequest,
			`{"status":false,"errors":[{"id":"UnknownBlueprint","msg":"Unknown blueprint name: http-server"}]}`,
			nil,
			[]string{"build_id", "warnings"},
		},
		{
			false,
			"POST",
			"/api/v0/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			http.StatusOK,
			`{"status": true}`,
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","upload":{"image_name":"test_upload","provider":"aws","settings":{"region":"frankfurt","accessKeyID":"accesskey","secretAccessKey":"secretkey","bucket":"clay","key":"imagekey"}}}`, test_distro.TestImageTypeName),
			http.StatusOK,
			`{"status": true}`,
			expectedComposeLocalAndAws,
			[]string{"build_id", "warnings"},
		},
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test-distro-2","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			http.StatusOK,
			`{"status": true}`,
			expectedComposeGoodDistro,
			[]string{"build_id", "warnings"},
		},
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test-fedora-1","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			http.StatusBadRequest,
			`{"status": false,"errors":[{"id":"DistroError", "msg":"Unknown distribution: fedora-1"}]}`,
			nil,
			[]string{"build_id", "warnings"},
		},
		{
			false,
			"POST",
			"/api/v1/compose",
			`{"blueprint_name": "test-distro-2","compose_type": "imaginary_type","branch": "master"}`,
			http.StatusBadRequest,
			`{"status": false,"errors":[{"id":"ComposeError", "msg":"Failed to get compose type \"imaginary_type\": invalid image type: imaginary_type"}]}`,
			nil,
			[]string{"build_id", "warnings"},
		},

		// === OSTree params ===
		// Ref + Parent = error (parent without URL)
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"refid","parent":"parentid","url":""}}`, test_distro.TestImageTypeOSTree),
			http.StatusBadRequest,
			`{"status": false, "errors":[{"id":"OSTreeOptionsError","msg":"ostree parent ref specified, but no URL to retrieve it"}]}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// Valid Ref + URL = OK
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"%s","parent":"","url":"%s"}}`, test_distro.TestImageTypeOSTree, ostreeRepoOther.OSTreeRef, ostreeRepoOther.Server.URL),
			http.StatusOK,
			`{"status": true}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// Ref + invalid URL = error
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"whatever","parent":"","url":"invalid-url"}}`, test_distro.TestImageTypeOSTree),
			http.StatusBadRequest,
			`{"status":false,"errors":[{"id":"OSTreeOptionsError","msg":"error sending request to ostree repository \"invalid-url/refs/heads/whatever\": Get \"invalid-url/refs/heads/whatever\": unsupported protocol scheme \"\""}]}`,
			nil,
			[]string{"build_id", "warnings"},
		},
		// Bad Ref + URL = error
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"/bad/ref","parent":"","url":"http://ostree/"}}`, test_distro.TestImageTypeOSTree),
			http.StatusBadRequest,
			`{"status":false,"errors":[{"id":"OSTreeOptionsError","msg":"Invalid ostree ref \"/bad/ref\""}]}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// Incorrect Ref + URL = the parameters are okay, but the ostree repo returns 404
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"%s","parent":"","url":"%s"}}`, test_distro.TestImageTypeOSTree, "the/wrong/ref", ostreeRepoDefault.Server.URL),
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"OSTreeOptionsError","msg":"ostree repository \"%s/refs/heads/the/wrong/ref\" returned status: 404 Not Found"}]}`, ostreeRepoDefault.Server.URL),
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// Ref + Parent + URL = OK
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"%s","parent":"%s","url":"%s"}}`, test_distro.TestImageTypeOSTree, "the/new/ref", ostreeRepoOther.OSTreeRef, ostreeRepoOther.Server.URL),
			http.StatusOK,
			`{"status":true}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// Parent + URL = OK
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"","parent":"%s","url":"%s"}}`, test_distro.TestImageTypeOSTree, ostreeRepoDefault.OSTreeRef, ostreeRepoDefault.Server.URL),
			http.StatusOK,
			`{"status":true}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
		// URL only = OK (uses default ref, so we need to specify URL for ostree repo with default ref)
		{
			false,
			"POST",
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type":"%s","branch":"master","ostree":{"ref":"","parent":"","url":"%s"}}`, test_distro.TestImageTypeOSTree, ostreeRepoDefault.Server.URL),
			http.StatusOK,
			`{"status":true}`,
			expectedComposeOSTree,
			[]string{"build_id", "warnings"},
		},
	}

	tempdir := t.TempDir()
	for _, c := range cases {
		api, s := createWeldrAPI(tempdir, rpmmd_mock.NoComposesFixture)
		test.TestRoute(t, api, c.External, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, c.IgnoreFields...)

		if c.ExpectedStatus != http.StatusOK {
			continue
		}

		composes := s.GetAllComposes()

		require.Equalf(t, 1, len(composes), "%s: bad compose count in store", c.Path)

		// I have no idea how to get the compose in better way
		var composeStruct store.Compose
		for _, c := range composes {
			composeStruct = c
			break
		}

		require.NotNilf(t, composeStruct.ImageBuild.Manifest, "%s: the compose in the store did not contain a blueprint", c.Path)

		if diff := cmp.Diff(composeStruct, *c.ExpectedCompose, test.IgnoreDates(), test.IgnoreUuids(), test.Ignore("Targets.Options.Location"), test.CompareImageTypes()); diff != "" {
			t.Errorf("%s: compose in store isn't the same as expected, diff:\n%s", c.Path, diff)
		}
	}
}

func TestComposeDelete(t *testing.T) {
	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	var cases = []struct {
		Path               string
		ExpectedJSON       string
		ExpectedIDsInStore []string
	}{
		{"/api/v0/compose/delete/30000000-0000-0000-0000-000000000002", `{"uuids":[{"uuid":"30000000-0000-0000-0000-000000000002","status":true}],"errors":[]}`, []string{"30000000-0000-0000-0000-000000000000", "30000000-0000-0000-0000-000000000001", "30000000-0000-0000-0000-000000000003", "30000000-0000-0000-0000-000000000004"}},
		{"/api/v0/compose/delete/30000000-0000-0000-0000-000000000002,30000000-0000-0000-0000-000000000003", `{"uuids":[{"uuid":"30000000-0000-0000-0000-000000000002","status":true},{"uuid":"30000000-0000-0000-0000-000000000003","status":true}],"errors":[]}`, []string{"30000000-0000-0000-0000-000000000000", "30000000-0000-0000-0000-000000000001", "30000000-0000-0000-0000-000000000004"}},
		{"/api/v0/compose/delete/30000000-0000-0000-0000-000000000003,30000000-0000-0000-0000-000000000000", `{"uuids":[{"uuid":"30000000-0000-0000-0000-000000000003","status":true}],"errors":[{"id":"BuildInWrongState","msg":"Compose 30000000-0000-0000-0000-000000000000 is not in FINISHED or FAILED."}]}`, []string{"30000000-0000-0000-0000-000000000000", "30000000-0000-0000-0000-000000000001", "30000000-0000-0000-0000-000000000002", "30000000-0000-0000-0000-000000000004"}},
		{"/api/v0/compose/delete/30000000-0000-0000-0000-000000000003,30000000-0000-0000-0000", `{"uuids":[{"uuid":"30000000-0000-0000-0000-000000000003","status":true}],"errors":[{"id":"UnknownUUID","msg":"30000000-0000-0000-0000 is not a valid uuid"}]}`, []string{"30000000-0000-0000-0000-000000000000", "30000000-0000-0000-0000-000000000001", "30000000-0000-0000-0000-000000000002", "30000000-0000-0000-0000-000000000004"}},
		{"/api/v0/compose/delete/30000000-0000-0000-0000-000000000003,42000000-0000-0000-0000-000000000000", `{"uuids":[{"uuid":"30000000-0000-0000-0000-000000000003","status":true}],"errors":[{"id":"UnknownUUID","msg":"compose 42000000-0000-0000-0000-000000000000 doesn't exist"}]}`, []string{"30000000-0000-0000-0000-000000000000", "30000000-0000-0000-0000-000000000001", "30000000-0000-0000-0000-000000000002", "30000000-0000-0000-0000-000000000004"}},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, s := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, false, "DELETE", c.Path, "", http.StatusOK, c.ExpectedJSON)

		idsInStore := []string{}

		for id := range s.GetAllComposes() {
			idsInStore = append(idsInStore, id.String())
		}

		require.ElementsMatch(t, c.ExpectedIDsInStore, idsInStore, "%s: composes in store are different", c.Path)
	}
}

func TestComposeStatus(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/status/30000000-0000-0000-0000-000000000000,30000000-0000-0000-0000-000000000002", ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000000","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","job_created":1574857140},{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/status/*", ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000000","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","job_created":1574857140},{"id":"30000000-0000-0000-0000-000000000001","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"RUNNING","job_created":1574857140,"job_started":1574857140},{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000003","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FAILED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/status/*?name=test", ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000000","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","job_created":1574857140},{"id":"30000000-0000-0000-0000-000000000001","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"RUNNING","job_created":1574857140,"job_started":1574857140},{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000003","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FAILED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/status/*?status=FINISHED", ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", fmt.Sprintf("/api/v0/compose/status/*?type=%s", test_distro.TestImageTypeName), ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000000","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","job_created":1574857140},{"id":"30000000-0000-0000-0000-000000000001","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"RUNNING","job_created":1574857140,"job_started":1574857140},{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000003","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FAILED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/status/30000000-0000-0000-0000-000000000000", ``, http.StatusOK, fmt.Sprintf(`{"uuids":[{"id":"30000000-0000-0000-0000-000000000000","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","job_created":1574857140,"uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"WAITING","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]}]}`, test_distro.TestImageTypeName)},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, false, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, "id", "job_created", "job_started")
	}
}

func TestComposeInfo(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/info/30000000-0000-0000-0000-000000000000", ``, http.StatusOK, fmt.Sprintf(`{"id":"30000000-0000-0000-0000-000000000000","config":"","blueprint":{"name":"test","description":"","distro":"","version":"0.0.0","packages":[],"modules":[],"groups":[]},"commit":"","deps":{"packages":[]},"compose_type":"%s","queue_status":"WAITING","image_size":0}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/info/30000000-0000-0000-0000-000000000000", ``, http.StatusOK, fmt.Sprintf(`{"id":"30000000-0000-0000-0000-000000000000","config":"","blueprint":{"name":"test","description":"","distro":"","version":"0.0.0","packages":[],"modules":[],"groups":[]},"commit":"","deps":{"packages":[]},"compose_type":"%s","queue_status":"WAITING","image_size":0,"uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"WAITING","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/info/30000000-0000-0000-0000", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownUUID","msg":"30000000-0000-0000-0000 is not a valid build uuid"}]}`},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/info/42000000-0000-0000-0000-000000000000", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownUUID","msg":"42000000-0000-0000-0000-000000000000 is not a valid build uuid"}]}`},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, false, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestComposeLogs(t *testing.T) {
	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	var successCases = []struct {
		Path                       string
		ExpectedContentDisposition string
		ExpectedContentType        string
		ExpectedFileName           string
		ExpectedFileContent        string
	}{
		{"/api/v0/compose/logs/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002-logs.tar", "application/x-tar", "logs/osbuild.log", "The compose result is empty.\n"},
		{"/api/v1/compose/logs/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002-logs.tar", "application/x-tar", "logs/osbuild.log", "The compose result is empty.\n"},
		{"/api/v0/compose/metadata/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002-metadata.tar", "application/x-tar", "30000000-0000-0000-0000-000000000002.json", `{"version":"","pipelines":[],"sources":{}}`},
		{"/api/v1/compose/metadata/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002-metadata.tar", "application/x-tar", "30000000-0000-0000-0000-000000000002.json", `{"version":"","pipelines":[],"sources":{}}`},
		{"/api/v0/compose/results/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002.tar", "application/x-tar", "30000000-0000-0000-0000-000000000002.json", `{"version":"","pipelines":[],"sources":{}}`},
		{"/api/v1/compose/results/30000000-0000-0000-0000-000000000002", "attachment; filename=30000000-0000-0000-0000-000000000002.tar", "application/x-tar", "30000000-0000-0000-0000-000000000002.json", `{"version":"","pipelines":[],"sources":{}}`},
	}

	tempdir := t.TempDir()

	for _, c := range successCases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)

		response := test.SendHTTP(api, false, "GET", c.Path, "")
		require.Equalf(t, http.StatusOK, response.StatusCode, "%s: unexpected status code", c.Path)
		require.Equalf(t, c.ExpectedContentDisposition, response.Header.Get("content-disposition"), "%s: header mismatch", c.Path)
		require.Equalf(t, c.ExpectedContentType, response.Header.Get("content-type"), "%s: header mismatch", c.Path)

		tr := tar.NewReader(response.Body)
		h, err := tr.Next()

		require.NoErrorf(t, err, "untarring failed with error")
		require.Falsef(t, h.ModTime.After(time.Now()), "ModTime cannot be in the future")
		require.Equalf(t, c.ExpectedFileName, h.Name, "%s: unexpected file name", c.Path)

		var buffer bytes.Buffer

		// vulnerability already tested
		/* #nosec G110 */
		_, err = io.Copy(&buffer, tr)
		require.NoErrorf(t, err, "cannot copy untar result")
		require.Equalf(t, c.ExpectedFileContent, buffer.String(), "%s: unexpected log content", c.Path)
	}

	var failureCases = []struct {
		Path         string
		ExpectedJSON string
	}{
		{"/api/v1/compose/logs/30000000-0000-0000-0000", `{"status":false,"errors":[{"id":"UnknownUUID","msg":"30000000-0000-0000-0000 is not a valid build uuid"}]}`},
		{"/api/v1/compose/logs/42000000-0000-0000-0000-000000000000", `{"status":false,"errors":[{"id":"UnknownUUID","msg":"Compose 42000000-0000-0000-0000-000000000000 doesn't exist"}]}`},
		{"/api/v1/compose/logs/30000000-0000-0000-0000-000000000000", `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000000 not in FINISHED or FAILED state."}]}`},
		{"/api/v1/compose/metadata/30000000-0000-0000-0000-000000000000", `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000000 is in wrong state: WAITING"}]}`},
		{"/api/v1/compose/results/30000000-0000-0000-0000-000000000000", `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000000 is in wrong state: WAITING"}]}`},
		{"/api/v1/compose/metadata/30000000-0000-0000-0000-000000000001", `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000001 is in wrong state: RUNNING"}]}`},
		{"/api/v1/compose/results/30000000-0000-0000-0000-000000000001", `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000001 is in wrong state: RUNNING"}]}`},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	for _, c := range failureCases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, false, "GET", c.Path, "", http.StatusBadRequest, c.ExpectedJSON)
	}
}

func TestComposeLog(t *testing.T) {
	var cases = []struct {
		Fixture          rpmmd_mock.FixtureGenerator
		Method           string
		Path             string
		ExpectedStatus   int
		ExpectedResponse string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/log/30000000-0000-0000-0000-000000000000", http.StatusOK, `{"status":false,"errors":[{"id":"BuildInWrongState","msg":"Build 30000000-0000-0000-0000-000000000000 has not started yet. No logs to view."}]}` + "\n"},
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/log/30000000-0000-0000-0000-000000000001", http.StatusOK, `Build 30000000-0000-0000-0000-000000000001 is still running.` + "\n"},
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/log/30000000-0000-0000-0000-000000000002", http.StatusOK, `The compose result is empty.` + "\n"},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/log/30000000-0000-0000-0000-000000000002", http.StatusOK, `The compose result is empty.` + "\n"},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/log/30000000-0000-0000-0000", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownUUID","msg":"30000000-0000-0000-0000 is not a valid build uuid"}]}` + "\n"},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/log/42000000-0000-0000-0000-000000000000", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownUUID","msg":"Compose 42000000-0000-0000-0000-000000000000 doesn't exist"}]}` + "\n"},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestNonJsonRoute(t, api, false, "GET", c.Path, "", c.ExpectedStatus, c.ExpectedResponse)
	}
}

func TestComposeQueue(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/queue", ``, http.StatusOK, fmt.Sprintf(`{"new":[{"blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING"}],"run":[{"blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"RUNNING"}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/queue", ``, http.StatusOK, fmt.Sprintf(`{"new":[{"blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"WAITING","uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"WAITING","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]}],"run":[{"blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"RUNNING"}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.NoComposesFixture, "GET", "/api/v0/compose/queue", ``, http.StatusOK, `{"new":[],"run":[]}`},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, false, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, "id", "job_created", "job_started")
	}
}

func TestComposeFinished(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/finished", ``, http.StatusOK, fmt.Sprintf(`{"finished":[{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/finished", ``, http.StatusOK, fmt.Sprintf(`{"finished":[{"id":"30000000-0000-0000-0000-000000000002","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140,"uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"FINISHED","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]},{"id":"30000000-0000-0000-0000-000000000004","blueprint":"test","version":"0.0.0","compose_type":"%[1]s","image_size":0,"queue_status":"FINISHED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140,"uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"FINISHED","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.NoComposesFixture, "GET", "/api/v0/compose/finished", ``, http.StatusOK, `{"finished":[]}`},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, false, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, "id", "job_created", "job_started")
	}
}

func TestComposeFailed(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "GET", "/api/v0/compose/failed", ``, http.StatusOK, fmt.Sprintf(`{"failed":[{"id":"30000000-0000-0000-0000-000000000003","blueprint":"test","version":"0.0.0","compose_type":"%s","image_size":0,"queue_status":"FAILED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.BaseFixture, "GET", "/api/v1/compose/failed", ``, http.StatusOK, fmt.Sprintf(`{"failed":[{"id":"30000000-0000-0000-0000-000000000003","blueprint":"test","version":"0.0.0","compose_type":"%s","image_size":0,"queue_status":"FAILED","job_created":1574857140,"job_started":1574857140,"job_finished":1574857140,"uploads":[{"uuid":"10000000-0000-0000-0000-000000000000","status":"FAILED","provider_name":"aws","image_name":"awsimage","creation_time":1574857140,"settings":{"region":"frankfurt","bucket":"clay","key":"imagekey"}}]}]}`, test_distro.TestImageTypeName)},
		{rpmmd_mock.NoComposesFixture, "GET", "/api/v0/compose/failed", ``, http.StatusOK, `{"failed":[]}`},
	}

	if len(os.Getenv("OSBUILD_COMPOSER_TEST_EXTERNAL")) > 0 {
		t.Skip("This test is for internal testing only")
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, false, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, "id", "job_created", "job_started")
	}
}

func TestSourcesNew(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"POST", "/api/v0/projects/source/new", ``, http.StatusBadRequest, `{"errors": [{"id": "ProjectsError","msg": "Missing source"}],"status":false}`},
		// Bad JSON, missing quote after name
		{"POST", "/api/v0/projects/source/new", `{"name: "fish","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "ProjectsError","msg": "Problem parsing POST body: invalid character 'f' after object key"}],"status":false}`},
		{"POST", "/api/v0/projects/source/new", `{"name": "fish","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v0/projects/source/new", `{"url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "ProjectsError","msg": "Problem parsing POST body: 'name' field is missing from request"}],"status":false}`},
		{"POST", "/api/v0/projects/source/new", `{"name": "fish", "type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "ProjectsError","msg": "Problem parsing POST body: 'url' field is missing from request"}],"status":false}`},
		{"POST", "/api/v0/projects/source/new", `{"name": "fish", "url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "ProjectsError","msg": "Problem parsing POST body: 'type' field is missing from request"}],"status":false}`},
		{"POST", "/api/v0/projects/source/new", `{"name": "test-id", "url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "SystemSource","msg": "test-id is a system source, it cannot be changed."}],"status":false}`},
		{"POST", "/api/v1/projects/source/new", `{"id": "test-id", "name": "test system repo", "url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`, http.StatusBadRequest, `{"errors": [{"id": "SystemSource","msg": "test-id is a system source, it cannot be changed."}],"status":false}`},
		{"POST", "/api/v1/projects/source/new", `{"id": "fish","name":"fish repo","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false,"distros":["test-distro", "test-distro-2"]}`, http.StatusOK, `{"status":true}`},
		{"POST", "/api/v1/projects/source/new", `{"id": "fish","name":"fish repo","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false,"distros":["fedora-1"]}`, http.StatusBadRequest, `{"status":false, "errors":[{"id":"ProjectsError", "msg":"Invalid distributions: fedora-1"}]}`},
		{"POST", "/api/v1/projects/source/new", `{"id": "fish","name":"fish repo","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": true,"check_repogpg":true,"gpgkeys": ["https://repourl/path/to/key.pub"]}`, http.StatusOK, `{"status":true}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, true, "DELETE", "/api/v0/projects/source/delete/fish", ``)
	}
}

func TestSourcesNewTomlV0(t *testing.T) {
	tempdir := t.TempDir()

	sources := []string{`
name = "fish"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `[fish]
name = "fish"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`}
	for _, source := range sources {
		req := httptest.NewRequest("POST", "/api/v0/projects/source/new", bytes.NewReader([]byte(source)))
		req.Header.Set("Content-Type", "text/x-toml")
		recorder := httptest.NewRecorder()

		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		api.ServeHTTP(recorder, req)

		r := recorder.Result()
		require.Equal(t, http.StatusOK, r.StatusCode)

		test.SendHTTP(api, true, "DELETE", "/api/v0/projects/source/delete/fish", ``)
	}
}

// Empty TOML, and invalid TOML should return an error
func TestSourcesNewWrongTomlV0(t *testing.T) {
	tempdir := t.TempDir()

	sources := []string{``, `
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`}
	for _, source := range sources {
		req := httptest.NewRequest("POST", "/api/v0/projects/source/new", bytes.NewReader([]byte(source)))
		req.Header.Set("Content-Type", "text/x-toml")
		recorder := httptest.NewRecorder()

		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		api.ServeHTTP(recorder, req)

		r := recorder.Result()
		require.Equal(t, http.StatusBadRequest, r.StatusCode)
	}
}

// TestSourcesNewTomlV1 tests the v1 sources API with id and name
func TestSourcesNewTomlV1(t *testing.T) {
	tempdir := t.TempDir()

	sources := []string{`
id = "fish"
name = "fish or cut bait"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `[fish]
name = "fish or cut bait"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `[fish]
id = "fish"
name = "fish or cut bait"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `id = "fish"
name = "fish or cut bait"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = true
check_repogpg = true
gpgkeys = ['''-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.10 (GNU/Linux)

mQENBEt+xXMBCACkA1ZtcO4H7ZUG/0aL4RlZIozsorXzFrrTAsJEHvdy+rHCH3xR
cFz6IMbfCOdV+oKxlDP7PS0vWKfqxwkenOUut5o9b32uDdFMW4IbFXEQ94AuSQpS
jo8PlVMm/51pmmRxdJzyPnr0YD38mVK6qUEYLI/4zXSgFk493GT8Y4m3N18O/+ye
PnOOItj7qbrCMASoBx1TG8Zdg8ufehMnfb85x4xxAebXkqJQpEVTjt4lj4p6BhrW
R+pIW/nBUrz3OsV7WwPKjSLjJtTJFxYX+RFSCqOdfusuysoOxpIHOx1WxjGUOB5j
fnhmq41nWXf8ozb58zSpjDrJ7jGQ9pdUpAtRABEBAAG0HkJyaWFuIEMuIExhbmUg
PGJjbEByZWRoYXQuY29tPokBOAQTAQIAIgUCS37FcwIbAwYLCQgHAwIGFQgCCQoL
BBYCAwECHgECF4AACgkQEX6MFo7+On9dgAf9Hi2K1MKcmLkDeSUIXkXIAw0nAzl2
UDGLWEdDqAgFxP6UaCVtOIRCr7z4EDOQoxD7mkdekbH2W5GcTO4h8MQBHYD9EkY7
H/lTKchlFfsmafOoA3Y/tDLPKu+OIfH9Mqn2Mf7wMYGrnWSRNKYgvC5zkMgkhoPU
mSPPHyBabsdS/Kg5ZAf43ac/MXY9V8Mk6zqbBlj6QYqjJ0nBD6vwozrDQ5gJtDUL
mQho13zPn4lBJl9YJVjcgRB2WbzgSZOln0DfV22Seai66vnr5NyaOIw5B9QLSNhN
EaPFswEDLKCsns9dkDuGFX52/Mt/i7JySvwhMBqHElPzWmwCHeY45M8gBYhGBBAR
AgAGBQJLfsbpAAoJECH7Y/6XEsLNuasAn0Q0jB4Ea/95EREUkCFTm9L6nOpAAJ9t
QzwGXhrLFZzOdRWYiWcCQbX5/7kBDQRLfsVzAQgAvN5jr95pJthv2w9co9/7omhM
5rAnr9WJfbMLLiUfPPUvpL24RGO6SKy03aiVTUjlaHc+cGqOciwnNKMCSt+noyG2
kNnAESTDtCivpsjonaFP8jA3TqL0QK+yzBRKJnMnLEY1nWE1FtkMRccXvzi0Z/XQ
VhiWQyTvDFoKtepBFrH9UqWbNHyki22aighumUsW01pcPH2ogSj+HR01r7SfI/y2
EkE6loHQfCDycHmlqYV+X6GZEvf1qu2+EHEQChsHIAxWyshsxM/ZPmx/8e5S3Xmj
l7h/6E9wcsIpvnf504sLX5j4Km9I5HgJSRxHxgRPpqJ2/XiClAJanO5gCw0RdQAR
AQABiQEfBBgBAgAJBQJLfsVzAhsMAAoJEBF+jBaO/jp/SqEH/iArzrfVOhZQGuy1
KmG0+/FdJGqAEHP5HWpsaeYJok1VmhTPZd4IVFBz/bGJYyvsrPU0pJ6QLkdGxNnb
KulJocgkW5MKEL/CRc54ESKwYngigmbY4qLwhS+gB3BJg1TvoHD810MSj4wdxNNo
6JQmFmuoDsLRwaRYbKQDz95XXoGQtmV1o57T05WkLuC5OmHqnWv3rggVC8madpUJ
moUUvUWgU1qyXe3PrgMGFOibWIl7lPZ08nzKXBRvSK/xoTGxl+570AevfVHMu5Uk
Yu2U6D6/DYohtTYp0s1ekS5KQkCJM7lfqecDsQhfVfOfR0w4aF8k8u3HmWdOfUz+
9+2ZsBo=
=myjM
-----END PGP PUBLIC KEY BLOCK-----''']
`}
	for _, source := range sources {
		req := httptest.NewRequest("POST", "/api/v1/projects/source/new", bytes.NewReader([]byte(source)))
		req.Header.Set("Content-Type", "text/x-toml")
		recorder := httptest.NewRecorder()

		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		api.ServeHTTP(recorder, req)

		r := recorder.Result()
		require.Equal(t, http.StatusOK, r.StatusCode)

		test.SendHTTP(api, true, "DELETE", "/api/v1/projects/source/delete/fish", ``)
	}
}

func TestSourcesInfoTomlV1(t *testing.T) {
	source := `
id = "fish"
name = "fish"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
rhsm = true
`

	sourceStr := `{"check_gpg":false,"check_repogpg":false,"check_ssl":false,"id":"fish","name":"fish","rhsm":true,"system":false,"type":"yum-baseurl","url":"https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"}`

	req := httptest.NewRequest("POST", "/api/v1/projects/source/new", bytes.NewReader([]byte(source)))
	req.Header.Set("Content-Type", "text/x-toml")
	recorder := httptest.NewRecorder()

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	api.ServeHTTP(recorder, req)

	r := recorder.Result()
	require.Equal(t, http.StatusOK, r.StatusCode)
	test.TestRoute(t, api, true, "GET", "/api/v1/projects/source/info/fish", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
	test.TestRoute(t, api, true, "GET", "/api/v1/projects/source/info/fish?format=json", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
}

func TestSourcesInfoGPGKeysV1(t *testing.T) {
	sourceStr := `{"id":"fish","name":"fish repo","type":"yum-baseurl","url":"https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","check_gpg":true,"check_repogpg":true,"check_ssl":false,"gpgkeys":["https://repourl/path/to/key.pub"],"rhsm":false,"system":false}`

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	test.SendHTTP(api, true, "POST", "/api/v1/projects/source/new", sourceStr)
	test.TestRoute(t, api, true, "GET", "/api/v1/projects/source/info/fish", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
	test.TestRoute(t, api, true, "GET", "/api/v1/projects/source/info/fish?format=json", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
}

// TestSourcesNewWrongTomlV1 Tests that Empty TOML, and invalid TOML should return an error
func TestSourcesNewWrongTomlV1(t *testing.T) {
	tempdir := t.TempDir()

	sources := []string{``, `
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `
[fish]
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`, `
id = "fish"
url = "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/"
type = "yum-baseurl"
check_ssl = false
check_gpg = false
`}
	for _, source := range sources {
		req := httptest.NewRequest("POST", "/api/v1/projects/source/new", bytes.NewReader([]byte(source)))
		req.Header.Set("Content-Type", "text/x-toml")
		recorder := httptest.NewRecorder()

		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		api.ServeHTTP(recorder, req)

		r := recorder.Result()
		require.Equal(t, http.StatusBadRequest, r.StatusCode)
	}
}

func TestSourcesInfo(t *testing.T) {
	sourceStr := `{"name":"fish","type":"yum-baseurl","url":"https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","check_gpg":false,"check_ssl":false,"system":false}`

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	test.SendHTTP(api, true, "POST", "/api/v0/projects/source/new", sourceStr)
	test.TestRoute(t, api, true, "GET", "/api/v0/projects/source/info/fish", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
	test.TestRoute(t, api, true, "GET", "/api/v0/projects/source/info/fish?format=json", ``, 200, `{"sources":{"fish":`+sourceStr+`},"errors":[]}`)
	test.TestRoute(t, api, true, "GET", "/api/v0/projects/source/info/fish?format=son", ``, 400, `{"status":false,"errors":[{"id":"InvalidChars","msg":"invalid format parameter: son"}]}`)
}

func TestSourcesInfoToml(t *testing.T) {
	sourceStr := `{"name":"fish","type":"yum-baseurl","url":"https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","check_gpg":false,"check_ssl":false,"system":false}`

	api, _ := createWeldrAPI(t.TempDir(), rpmmd_mock.BaseFixture)
	test.SendHTTP(api, true, "POST", "/api/v0/projects/source/new", sourceStr)

	req := httptest.NewRequest("GET", "/api/v0/projects/source/info/fish?format=toml", nil)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)
	resp := recorder.Result()

	var sources map[string]store.SourceConfig
	_, err := toml.NewDecoder(resp.Body).Decode(&sources)
	require.NoErrorf(t, err, "error decoding toml file")

	expected := map[string]store.SourceConfig{
		"fish": {
			Name: "fish",
			Type: "yum-baseurl",
			URL:  "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/",
		},
	}

	require.Equal(t, expected, sources)
}

func TestSourcesDelete(t *testing.T) {
	var cases = []struct {
		Method         string
		Path           string
		Body           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{"DELETE", "/api/v0/projects/source/delete/", ``, http.StatusNotFound, `{"status":false,"errors":[{"code":404,"id":"HTTPError","msg":"Not Found"}]}`},
		{"DELETE", "/api/v0/projects/source/delete/fish", ``, http.StatusOK, `{"status":true}`},
		{"DELETE", "/api/v0/projects/source/delete/unknown", ``, http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownSource","msg":"unknown is not a valid source."}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, rpmmd_mock.BaseFixture)
		test.SendHTTP(api, true, "POST", "/api/v0/projects/source/new", `{"name": "fish","url": "https://download.opensuse.org/repositories/shells:/fish:/release:/3/Fedora_29/","type": "yum-baseurl","check_ssl": false,"check_gpg": false}`)
		test.TestRoute(t, api, true, c.Method, c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON)
		test.SendHTTP(api, true, "DELETE", "/api/v0/projects/source/delete/fish", ``)
	}
}

func TestProjectsDepsolve(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.NonExistingPackage, "/api/v0/projects/depsolve/fash", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ProjectsError","msg":"BadRequest: DNF error occurred: MarkingErrors: Error occurred when marking packages for installation: Problems in request:\nmissing packages: fash"}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/depsolve/fish", http.StatusOK, `{"projects":[{"name":"dep-package3","epoch":7,"version":"3.0.3","release":"1.fc30","arch":"x86_64","check_gpg":true},{"name":"dep-package1","epoch":0,"version":"1.33","release":"2.fc30","arch":"x86_64","check_gpg":true},{"name":"dep-package2","epoch":0,"version":"2.9","release":"1.fc30","arch":"x86_64","check_gpg":true}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/depsolve/fish?distro=test-distro-2", http.StatusOK, `{"projects":[{"name":"dep-package3","epoch":7,"version":"3.0.3","release":"1.fc30","arch":"x86_64","check_gpg":true},{"name":"dep-package1","epoch":0,"version":"1.33","release":"2.fc30","arch":"x86_64","check_gpg":true},{"name":"dep-package2","epoch":0,"version":"2.9","release":"1.fc30","arch":"x86_64","check_gpg":true}]}`},
		{rpmmd_mock.BadDepsolve, "/api/v0/projects/depsolve/go2rpm", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ProjectsError","msg":"BadRequest: DNF error occurred: DepsolveError: There was a problem depsolving ['go2rpm']: \n Problem: conflicting requests\n  - nothing provides askalono-cli needed by go2rpm-1-4.fc31.noarch"}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/depsolve/fish?distro=fedora-1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestProjectsInfo(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownProject","msg":"No packages specified."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownProject","msg":"No packages specified."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/nonexistingpkg", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownProject","msg":"No packages have been found."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/*", http.StatusOK, projectsInfoResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/package2*,package16", http.StatusOK, projectsInfoFilteredResponse},
		{rpmmd_mock.BadFetch, "/api/v0/projects/info/badpackage1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ProjectsError","msg":"msg: DNF error occurred: FetchError: There was a problem when fetching packages."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/package16?distro=test-distro-2", http.StatusOK, projectsInfoPackage16Response},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/info/package16?distro=fedora-1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestModulesInfo(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "/api/v0/modules/info", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownModule","msg":"No packages specified."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/info/", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownModule","msg":"No packages specified."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/info/nonexistingpkg", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownModule","msg":"No packages have been found."}]}`},
		{rpmmd_mock.BadDepsolve, "/api/v0/modules/info/baddepsolve", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ModulesError","msg":"Cannot depsolve package package1: DNF error occurred: DepsolveError: There was a problem depsolving ['go2rpm']: \n Problem: conflicting requests\n  - nothing provides askalono-cli needed by go2rpm-1-4.fc31.noarch"}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/info/package2*,package16", http.StatusOK, modulesInfoFilteredResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/info/*", http.StatusOK, modulesInfoResponse},
		{rpmmd_mock.BadFetch, "/api/v0/modules/info/badpackage1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ModulesError","msg":"msg: DNF error occurred: FetchError: There was a problem when fetching packages."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v1/modules/info/package2*,package16", http.StatusOK, modulesInfoFilteredResponse},
		{rpmmd_mock.BaseFixture, "/api/v1/modules/info/package16?distro=test-distro-2", http.StatusOK, modulesInfoPackage16Response},
		{rpmmd_mock.BaseFixture, "/api/v1/modules/info/package16?distro=fedora-1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestProjectsList(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "/api/v0/projects/list", http.StatusOK, projectsListResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/list/", http.StatusOK, projectsListResponse},
		{rpmmd_mock.BadFetch, "/api/v0/projects/list/", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ProjectsError","msg":"msg: DNF error occurred: FetchError: There was a problem when fetching packages."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/list?offset=1&limit=1", http.StatusOK, projectsList1Response},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/list?distro=test-distro-2&offset=1&limit=1", http.StatusOK, projectsList1Response},
		{rpmmd_mock.BaseFixture, "/api/v0/projects/list?distro=fedora-1&offset=1&limit=1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestModulesList(t *testing.T) {
	var cases = []struct {
		Fixture        rpmmd_mock.FixtureGenerator
		Path           string
		ExpectedStatus int
		ExpectedJSON   string
	}{
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list", http.StatusOK, modulesListResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/", http.StatusOK, modulesListResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/nonexistingpkg", http.StatusBadRequest, `{"status":false,"errors":[{"id":"UnknownModule","msg":"No packages have been found."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/package2*,package16", http.StatusOK, modulesListFilteredResponse},
		{rpmmd_mock.BadFetch, "/api/v0/modules/list/badpackage1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"ModulesError","msg":"msg: DNF error occurred: FetchError: There was a problem when fetching packages."}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/package2*,package16?offset=1&limit=1", http.StatusOK, `{"total":4,"offset":1,"limit":1,"modules":[{"name":"package2","group_type":"rpm"}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/*", http.StatusOK, modulesListResponse},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/package2*,package16?distro=test-distro-2&offset=1&limit=1", http.StatusOK, `{"total":4,"offset":1,"limit":1,"modules":[{"name":"package2","group_type":"rpm"}]}`},
		{rpmmd_mock.BaseFixture, "/api/v0/modules/list/package2*,package16?distro=fedora-1&offset=1&limit=1", http.StatusBadRequest, `{"status":false,"errors":[{"id":"DistroError","msg":"Invalid distro: fedora-1"}]}`},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI(tempdir, c.Fixture)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestComposeTypes_ImageTypeDenylist(t *testing.T) {
	var cases = []struct {
		Path              string
		ImageTypeDenylist map[string][]string
		ExpectedStatus    int
		ExpectedJSON      string
	}{
		{
			"/api/v1/compose/types",
			map[string][]string{},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q},{"enabled":true, "name":%q}]}`, test_distro.TestImageTypeName, test_distro.TestImageType2Name),
		},
		{
			"/api/v1/compose/types?distro=test-distro-2",
			map[string][]string{},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q},{"enabled":true, "name":%q}]}`, test_distro.TestImageTypeName, test_distro.TestImageType2Name),
		},
		{
			"/api/v1/compose/types",
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName}},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q}]}`, test_distro.TestImageType2Name),
		},
		{
			"/api/v1/compose/types?distro=test-distro-2",
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName}},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q}]}`, test_distro.TestImageType2Name),
		},
		{
			"/api/v1/compose/types",
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName, test_distro.TestImageType2Name}},
			http.StatusOK,
			`{"types": null}`,
		},
		{
			"/api/v1/compose/types?distro=test-distro-2",
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName, test_distro.TestImageType2Name}},
			http.StatusOK,
			`{"types": null}`,
		},
		{
			"/api/v1/compose/types",
			map[string][]string{"*": {test_distro.TestImageTypeName}},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q}]}`, test_distro.TestImageType2Name),
		},
		{
			"/api/v1/compose/types",
			map[string][]string{"*": {test_distro.TestImageTypeName, test_distro.TestImageType2Name}},
			http.StatusOK,
			`{"types": null}`,
		},
		{
			"/api/v1/compose/types",
			map[string][]string{test_distro.TestDistro2Name: {"*"}},
			http.StatusOK,
			`{"types": null}`,
		},
		{
			"/api/v1/compose/types?distro=test-distro-2",
			map[string][]string{test_distro.TestDistroName: {"*"}},
			http.StatusOK,
			fmt.Sprintf(`{"types": [{"enabled":true, "name":%q}, {"enabled":true, "name":%q}]}`,
				test_distro.TestImageTypeName, test_distro.TestImageType2Name),
		},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, _ := createWeldrAPI2(tempdir, rpmmd_mock.BaseFixture, c.ImageTypeDenylist)
		test.TestRoute(t, api, true, "GET", c.Path, ``, c.ExpectedStatus, c.ExpectedJSON)
	}
}

func TestComposePOST_ImageTypeDenylist(t *testing.T) {
	arch, err := test_distro.New2().GetArch(test_distro.TestArch2Name)
	require.NoError(t, err)
	imgType, err := arch.GetImageType(test_distro.TestImageTypeName)
	require.NoError(t, err)
	imgType2, err := arch.GetImageType(test_distro.TestImageType2Name)
	require.NoError(t, err)
	manifest, _, err := imgType.Manifest(nil, distro.ImageOptions{}, nil, nil, nil, 0)
	require.NoError(t, err)

	expectedComposeLocal := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   imgType,
			Manifest:    manifest,
			Targets: []*target.Target{
				{
					ImageName: imgType.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType.Filename(),
						ExportName:     imgType.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}

	expectedComposeLocal2 := &store.Compose{
		Blueprint: &blueprint.Blueprint{
			Name:           "test",
			Version:        "0.0.0",
			Packages:       []blueprint.Package{},
			Modules:        []blueprint.Package{},
			Groups:         []blueprint.Group{},
			Customizations: nil,
		},
		ImageBuild: store.ImageBuild{
			QueueStatus: common.IBWaiting,
			ImageType:   imgType2,
			Manifest:    manifest,
			Targets: []*target.Target{
				{
					ImageName: imgType2.Filename(),
					OsbuildArtifact: target.OsbuildArtifact{
						ExportFilename: imgType2.Filename(),
						ExportName:     imgType2.Exports()[0],
					},
					Name:    target.TargetNameWorkerServer,
					Options: &target.WorkerServerTargetOptions{},
				},
			},
		},
		Packages: dnfjson_mock.BaseDeps(),
	}

	var cases = []struct {
		Path              string
		Body              string
		imageTypeDenylist map[string][]string
		ExpectedStatus    int
		ExpectedJSON      string
		ExpectedCompose   *store.Compose
		IgnoreFields      []string
	}{
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{},
			http.StatusOK,
			`{"status":true}`,
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageType2Name),
			map[string][]string{},
			http.StatusOK,
			`{"status": true}`,
			expectedComposeLocal2,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageTypeName, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageType2Name),
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName}},
			http.StatusOK,
			`{"status": true}`,
			expectedComposeLocal2,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName, test_distro.TestImageType2Name}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageTypeName, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageType2Name),
			map[string][]string{test_distro.TestDistro2Name: {test_distro.TestImageTypeName, test_distro.TestImageType2Name}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageType2Name, test_distro.TestDistro2Name),
			expectedComposeLocal2,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{"*": {test_distro.TestImageTypeName}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageTypeName, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{test_distro.TestDistro2Name: {"*"}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageTypeName, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageTypeName),
			map[string][]string{fmt.Sprintf("%s*", test_distro.TestDistroName): {fmt.Sprintf("%s*", test_distro.TestImageTypeName)}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageTypeName, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
		{
			"/api/v1/compose",
			fmt.Sprintf(`{"blueprint_name": "test","compose_type": "%s","branch": "master"}`, test_distro.TestImageType2Name),
			map[string][]string{fmt.Sprintf("%s*", test_distro.TestDistroName): {fmt.Sprintf("%s*", test_distro.TestImageTypeName)}},
			http.StatusBadRequest,
			fmt.Sprintf(`{"status":false,"errors":[{"id":"ComposeError","msg":"Failed to get compose type \"%[1]s\": image type \"%[1]s\" for distro \"%[2]s\" is denied by configuration"}]}`,
				test_distro.TestImageType2Name, test_distro.TestDistro2Name),
			expectedComposeLocal,
			[]string{"build_id", "warnings"},
		},
	}

	tempdir := t.TempDir()

	for _, c := range cases {
		api, s := createWeldrAPI2(tempdir, rpmmd_mock.NoComposesFixture, c.imageTypeDenylist)
		test.TestRoute(t, api, true, "POST", c.Path, c.Body, c.ExpectedStatus, c.ExpectedJSON, c.IgnoreFields...)

		if c.ExpectedStatus != http.StatusOK {
			continue
		}

		composes := s.GetAllComposes()
		require.Equalf(t, 1, len(composes), "%s: bad compose count in store", c.Path)

		var composeStruct store.Compose
		for _, c := range composes {
			composeStruct = c
			break
		}

		require.NotNilf(t, composeStruct.ImageBuild.Manifest, "%s: the compose in the store did not contain a blueprint", c.Path)

		if diff := cmp.Diff(composeStruct, *c.ExpectedCompose, test.IgnoreDates(), test.IgnoreUuids(), test.Ignore("Targets.Options.Location"), test.CompareImageTypes()); diff != "" {
			t.Errorf("%s: compose in store isn't the same as expected, diff:\n%s", c.Path, diff)
		}
	}
}

func TestMain(m *testing.M) {
	setupDNFJSON()
	defer os.RemoveAll(dnfjsonPath)
	code := m.Run()
	os.Exit(code)
}
