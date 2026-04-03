package main

import (
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

func testInstallerView() *runtimeview.View {
	prof, err := profile.Load(filepath.Join("..", "..", "profiles", "ascend-910b.yaml"))
	if err != nil {
		panic(err)
	}
	cfg, err := config.DefaultsFromProfile(prof)
	if err != nil {
		panic(err)
	}
	return runtimeview.New(cfg, prof)
}

func TestCopyBinaries_CopiesRuntimeAndHook(t *testing.T) {
	srcDir := t.TempDir()
	hostMount := t.TempDir()
	for name, content := range map[string]string{
		"accelerator-container-runtime": "runtime-binary",
		"accelerator-container-hook":    "hook-binary",
	} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}

	prevSource := installerBinarySource
	installerBinarySource = srcDir
	t.Cleanup(func() {
		installerBinarySource = prevSource
	})
	t.Setenv("HOST_MOUNT", hostMount)

	if err := copyBinaries("/usr/local/bin"); err != nil {
		t.Fatalf("copyBinaries returned error: %v", err)
	}

	for name, want := range map[string]string{
		"accelerator-container-runtime": "runtime-binary",
		"accelerator-container-hook":    "hook-binary",
	} {
		data, err := os.ReadFile(filepath.Join(hostMount, "usr/local/bin", name))
		if err != nil {
			t.Fatalf("reading copied %s: %v", name, err)
		}
		if string(data) != want {
			t.Fatalf("%s content = %q, want %q", name, string(data), want)
		}
	}
}

func TestWriteConfig_WritesConfigAndProfile(t *testing.T) {
	hostMount := t.TempDir()
	t.Setenv("HOST_MOUNT", hostMount)
	t.Setenv("ACCELERATOR_LOG_LEVEL", "debug")

	profilePath := filepath.Join(t.TempDir(), "active.yaml")
	profileContent := "metadata:\n  name: test-profile\n"
	if err := os.WriteFile(profilePath, []byte(profileContent), 0644); err != nil {
		t.Fatal(err)
	}

	view := testInstallerView()
	if err := writeConfig("/etc/accelerator-toolkit", "/opt/accelerator/bin", view, profilePath); err != nil {
		t.Fatalf("writeConfig returned error: %v", err)
	}

	configPath := filepath.Join(hostMount, "etc/accelerator-toolkit/config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
	}
	if cfg.HookPath != "/opt/accelerator/bin/accelerator-container-hook" {
		t.Fatalf("HookPath = %q, want %q", cfg.HookPath, "/opt/accelerator/bin/accelerator-container-hook")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}

	copiedProfile, err := os.ReadFile(filepath.Join(hostMount, "etc/accelerator-toolkit/profiles/active.yaml"))
	if err != nil {
		t.Fatalf("reading copied profile: %v", err)
	}
	if string(copiedProfile) != profileContent {
		t.Fatalf("copied profile = %q, want %q", string(copiedProfile), profileContent)
	}
}

func TestPatchContainerd_IsIdempotent(t *testing.T) {
	hostMount := t.TempDir()
	t.Setenv("HOST_MOUNT", hostMount)

	cfgPath := filepath.Join(hostMount, "etc/containerd/config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	initial := `[plugins."io.containerd.grpc.v1.cri".containerd]` + "\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	view := testInstallerView()
	if err := patchContainerd("/usr/local/bin", view); err != nil {
		t.Fatalf("first patchContainerd call returned error: %v", err)
	}
	if err := patchContainerd("/usr/local/bin", view); err != nil {
		t.Fatalf("second patchContainerd call returned error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading patched config: %v", err)
	}
	content := string(data)
	marker := `[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ascend-910b]`
	if strings.Count(content, marker) != 1 {
		t.Fatalf("runtime stanza count = %d, want 1\n%s", strings.Count(content, marker), content)
	}
	if !strings.Contains(content, `BinaryName = "/usr/local/bin/accelerator-container-runtime"`) {
		t.Fatalf("patched config missing BinaryName:\n%s", content)
	}
}

func TestLabelNode_PatchesProfileLabels(t *testing.T) {
	var (
		gotMethod string
		gotAuth   string
		gotBody   []byte
	)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	tokenPath := filepath.Join(t.TempDir(), "token")
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, []byte("unused"), 0644); err != nil {
		t.Fatal(err)
	}

	prevToken := serviceAccountToken
	prevCA := serviceAccountCA
	prevClientFactory := newKubernetesHTTPClient
	serviceAccountToken = tokenPath
	serviceAccountCA = caPath
	newKubernetesHTTPClient = func(_ *x509.CertPool) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		serviceAccountToken = prevToken
		serviceAccountCA = prevCA
		newKubernetesHTTPClient = prevClientFactory
	})

	t.Setenv("NODE_NAME", "kunlun-02")
	t.Setenv("KUBERNETES_SERVICE_HOST", parsed.Hostname())
	t.Setenv("KUBERNETES_SERVICE_PORT", parsed.Port())

	if err := labelNode(testInstallerView()); err != nil {
		t.Fatalf("labelNode returned error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPatch)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
	if !strings.Contains(string(gotBody), `"accelerator":"huawei-Ascend910"`) {
		t.Fatalf("patch body missing profile label: %s", string(gotBody))
	}
}
