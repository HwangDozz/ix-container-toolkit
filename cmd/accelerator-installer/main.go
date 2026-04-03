// installer installs accelerator-toolkit binaries and configures the container runtime
// on a Kubernetes node. It is designed to run as an init container in a
// privileged DaemonSet with hostPath mounts.
//
// What it does:
//  1. Copies accelerator-container-runtime and accelerator-container-hook to the host at
//     /usr/local/bin/ (configurable via environment variables).
//  2. Writes the accelerator-toolkit config to /etc/accelerator-toolkit/config.json.
//  3. Patches the containerd config to register accelerator-container-runtime under the
//     runtime handler declared by the active profile.
//  4. Labels the current node with the labels declared by the active profile
//     via the Kubernetes API so that the DaemonSet nodeSelector can match it.
//  5. (Optional) Restarts containerd via systemd dbus if RESTART_CONTAINERD=true.
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

const (
	defaultHostBinDir    = "/usr/local/bin"
	defaultHostConfigDir = "/etc/accelerator-toolkit"
	containerdConfigPath = "/etc/containerd/config.toml"
)

var (
	log                     = logrus.New()
	installerBinarySource   = "/usr/local/bin"
	serviceAccountToken     = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCA        = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	newKubernetesHTTPClient = func(caPool *x509.CertPool) *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: caPool},
			},
		}
	}
)

func main() {
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log.Info("accelerator-toolkit installer starting")

	hostBinDir := envOr("HOST_BIN_DIR", defaultHostBinDir)
	hostConfigDir := envOr("HOST_CONFIG_DIR", defaultHostConfigDir)
	profilePath := config.ResolveProfilePath(os.Getenv("ACCELERATOR_PROFILE_FILE"))

	view, err := runtimeview.Load("", profilePath)
	if err != nil {
		log.WithError(err).Fatal("failed to load runtime view")
	}
	log.WithFields(logrus.Fields{
		"profile": view.Profile().Metadata.Name,
		"path":    profilePath,
	}).Info("loaded active profile")

	steps := []struct {
		name string
		fn   func() error
	}{
		{"copy binaries", func() error { return copyBinaries(hostBinDir) }},
		{"write config", func() error { return writeConfig(hostConfigDir, hostBinDir, view, profilePath) }},
		{"patch containerd", func() error { return patchContainerd(hostBinDir, view) }},
		{"label node", func() error { return labelNode(view) }},
		{"restart containerd", restartContainerd},
	}

	for _, step := range steps {
		log.WithField("step", step.name).Info("running")
		if err := step.fn(); err != nil {
			log.WithError(err).Fatalf("step %q failed", step.name)
		}
	}

	log.Info("accelerator-toolkit installation complete")
}

// copyBinaries copies the hook and runtime binaries from the installer image
// to the host filesystem (via a hostPath volume typically mounted at /host).
func copyBinaries(hostBinDir string) error {
	// When running as a DaemonSet, the host rootfs is typically mounted at /host.
	hostMount := envOr("HOST_MOUNT", "/host")

	binaries := []string{"accelerator-container-runtime", "accelerator-container-hook"}
	for _, bin := range binaries {
		// The installer image ships the binaries at /usr/local/bin/<name>.
		src := filepath.Join(installerBinarySource, bin)
		dst := filepath.Join(hostMount, hostBinDir, bin)

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := copyFile(src, dst, 0755); err != nil {
			return fmt.Errorf("copying %s: %w", bin, err)
		}
		log.WithFields(logrus.Fields{"src": src, "dst": dst}).Info("binary copied")
	}
	return nil
}

// writeConfig writes the accelerator-toolkit config.json to the host.
func writeConfig(hostConfigDir, hostBinDir string, view *runtimeview.View, profilePath string) error {
	hostMount := envOr("HOST_MOUNT", "/host")

	cfg := *view.Config()
	cfg.HookPath = filepath.Join(hostBinDir, "accelerator-container-hook")

	if v := os.Getenv("ACCELERATOR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Join(hostMount, hostConfigDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", cfgPath, err)
	}
	log.WithField("path", cfgPath).Info("config written")

	profileDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", profileDir, err)
	}
	hostProfilePath := filepath.Join(profileDir, "active.yaml")
	if err := copyFile(profilePath, hostProfilePath, 0644); err != nil {
		return fmt.Errorf("copying profile to host: %w", err)
	}
	log.WithField("path", hostProfilePath).Info("profile copied to host")

	return nil
}

// patchContainerd adds the profile-declared accelerator runtime to containerd's config.toml.
// It appends a [plugins."io.containerd.grpc.v1.cri"…] stanza if one doesn't
// already exist, so the operation is idempotent.
func patchContainerd(hostBinDir string, view *runtimeview.View) error {
	hostMount := envOr("HOST_MOUNT", "/host")
	cfgPath := filepath.Join(hostMount, containerdConfigPath)

	data, err := os.ReadFile(filepath.Clean(cfgPath))
	if err != nil {
		if os.IsNotExist(err) {
			log.WithField("path", cfgPath).Warn("containerd config not found, skipping patch")
			return nil
		}
		return fmt.Errorf("reading %s: %w", cfgPath, err)
	}

	content := string(data)
	runtimeBin := filepath.Join(hostBinDir, "accelerator-container-runtime")
	handlerName := view.HandlerName()
	marker := fmt.Sprintf(`[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.%s]`, handlerName)

	if strings.Contains(content, marker) {
		log.Info("containerd already configured for this accelerator runtime, skipping")
		return nil
	}

	stanza := fmt.Sprintf(`
# --- accelerator-toolkit: runtime (auto-generated) ---
%s
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.%s.options]
    BinaryName = "%s"
# --- end accelerator-toolkit ---
`, marker, handlerName, runtimeBin)

	patched := content + stanza
	if err := os.WriteFile(cfgPath, []byte(patched), 0644); err != nil {
		return fmt.Errorf("writing patched %s: %w", cfgPath, err)
	}
	log.WithField("path", cfgPath).Info("containerd config patched")
	return nil
}

// restartContainerd restarts containerd via systemctl if RESTART_CONTAINERD=true.
func restartContainerd() error {
	if strings.ToLower(os.Getenv("RESTART_CONTAINERD")) != "true" {
		log.Info("RESTART_CONTAINERD not set to 'true', skipping restart")
		return nil
	}

	hostMount := envOr("HOST_MOUNT", "/host")
	systemctl := filepath.Join(hostMount, "usr/bin/systemctl")
	if _, err := os.Stat(systemctl); os.IsNotExist(err) {
		systemctl = "systemctl" // fall back to PATH
	}

	log.Info("restarting containerd")
	cmd := exec.Command(systemctl, "restart", "containerd")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemctl restart containerd: %w", err)
	}
	return nil
}

// copyFile copies src to dst with the given permissions.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// labelNode patches the current Kubernetes node with the labels declared by
// the active runtime view using the in-cluster ServiceAccount token.
// The node name is read from the NODE_NAME environment variable, which should
// be injected by the DaemonSet via the Downward API.
//
// If the Kubernetes API is unreachable or NODE_NAME is unset, a warning is
// logged and the step is skipped (non-fatal) so the rest of the installation
// can proceed.
func labelNode(view *runtimeview.View) error {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Warn("NODE_NAME not set, skipping node labeling (add it via Downward API in the DaemonSet)")
		return nil
	}

	token, err := os.ReadFile(serviceAccountToken)
	if err != nil {
		log.WithError(err).Warn("cannot read ServiceAccount token, skipping node labeling")
		return nil
	}

	caPool := x509.NewCertPool()
	if caData, err := os.ReadFile(serviceAccountCA); err == nil {
		caPool.AppendCertsFromPEM(caData)
	}

	apiHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	apiPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if apiHost == "" {
		apiHost = "kubernetes.default.svc"
	}
	if apiPort == "" {
		apiPort = "443"
	}

	labels := view.NodeLabels()

	// JSON Merge Patch: add node labels declared by the profile.
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling label patch: %w", err)
	}

	url := fmt.Sprintf("https://%s:%s/api/v1/nodes/%s", apiHost, apiPort, nodeName)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating PATCH request: %w", err)
	}
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))

	client := newKubernetesHTTPClient(caPool)
	resp, err := client.Do(req)
	if err != nil {
		log.WithError(err).Warn("PATCH node labels failed, skipping (non-fatal)")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.WithFields(logrus.Fields{
			"status": resp.StatusCode,
			"body":   string(respBody),
		}).Warn("PATCH node labels returned non-2xx, skipping (non-fatal)")
		return nil
	}

	log.WithFields(logrus.Fields{
		"node":   nodeName,
		"labels": labels,
	}).Info("node labeled")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
