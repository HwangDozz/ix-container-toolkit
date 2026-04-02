// installer installs ix-toolkit binaries and configures the container runtime
// on a Kubernetes node. It is designed to run as an init container in a
// privileged DaemonSet with hostPath mounts.
//
// What it does:
//  1. Copies ix-container-runtime and ix-container-hook to the host at
//     /usr/local/bin/ (configurable via environment variables).
//  2. Writes the ix-toolkit config to /etc/ix-toolkit/config.json.
//  3. Patches the containerd config to register ix-container-runtime as a
//     runtime class.
//  4. Labels the current node with iluvatar.ai/gpu=present via the Kubernetes
//     API (using the in-cluster ServiceAccount token) so that the DaemonSet
//     nodeSelector can match it.
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

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
)

const (
	defaultHostBinDir    = "/usr/local/bin"
	defaultHostConfigDir = "/etc/ix-toolkit"
	containerdConfigPath = "/etc/containerd/config.toml"
)

var log = logrus.New()

func main() {
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log.Info("ix-toolkit installer starting")

	hostBinDir := envOr("HOST_BIN_DIR", defaultHostBinDir)
	hostConfigDir := envOr("HOST_CONFIG_DIR", defaultHostConfigDir)

	steps := []struct {
		name string
		fn   func() error
	}{
		{"copy binaries", func() error { return copyBinaries(hostBinDir) }},
		{"write config", func() error { return writeConfig(hostConfigDir, hostBinDir) }},
		{"patch containerd", func() error { return patchContainerd(hostBinDir) }},
		{"label node", labelNode},
		{"restart containerd", restartContainerd},
	}

	for _, step := range steps {
		log.WithField("step", step.name).Info("running")
		if err := step.fn(); err != nil {
			log.WithError(err).Fatalf("step %q failed", step.name)
		}
	}

	log.Info("ix-toolkit installation complete")
}

// copyBinaries copies the hook and runtime binaries from the installer image
// to the host filesystem (via a hostPath volume typically mounted at /host).
func copyBinaries(hostBinDir string) error {
	// When running as a DaemonSet, the host rootfs is typically mounted at /host.
	hostMount := envOr("HOST_MOUNT", "/host")

	binaries := []string{"ix-container-runtime", "ix-container-hook"}
	for _, bin := range binaries {
		// The installer image ships the binaries at /usr/local/bin/<name>.
		src := filepath.Join("/usr/local/bin", bin)
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

// writeConfig writes the ix-toolkit config.json to the host.
func writeConfig(hostConfigDir, hostBinDir string) error {
	hostMount := envOr("HOST_MOUNT", "/host")

	cfg := config.Defaults()
	cfg.HookPath = filepath.Join(hostBinDir, "ix-container-hook")

	// Allow overrides from environment.
	if v := os.Getenv("IX_DRIVER_LIB_PATHS"); v != "" {
		cfg.Hook.DriverLibraryPaths = strings.Split(v, ":")
	}
	if v := os.Getenv("IX_DRIVER_BIN_PATHS"); v != "" {
		cfg.Hook.DriverBinaryPaths = strings.Split(v, ":")
	}
	if v := os.Getenv("IX_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
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
	return nil
}

// patchContainerd adds the ix runtime class to containerd's config.toml.
// It appends a [plugins."io.containerd.grpc.v1.cri"…] stanza if one doesn't
// already exist, so the operation is idempotent.
func patchContainerd(hostBinDir string) error {
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
	runtimeBin := filepath.Join(hostBinDir, "ix-container-runtime")
	marker := `[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix]`

	if strings.Contains(content, marker) {
		log.Info("containerd already configured for ix runtime, skipping")
		return nil
	}

	stanza := fmt.Sprintf(`
# --- ix-toolkit: Iluvatar GPU runtime (auto-generated) ---
%s
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.ix.options]
    BinaryName = "%s"
# --- end ix-toolkit ---
`, marker, runtimeBin)

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

// labelNode patches the current Kubernetes node with the label
// iluvatar.ai/gpu=present using the in-cluster ServiceAccount token.
// The node name is read from the NODE_NAME environment variable, which should
// be injected by the DaemonSet via the Downward API.
//
// If the Kubernetes API is unreachable or NODE_NAME is unset, a warning is
// logged and the step is skipped (non-fatal) so the rest of the installation
// can proceed.
func labelNode() error {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Warn("NODE_NAME not set, skipping node labeling (add it via Downward API in the DaemonSet)")
		return nil
	}

	// In-cluster credentials written by Kubernetes into every Pod.
	const (
		tokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
		caFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	)

	token, err := os.ReadFile(tokenFile)
	if err != nil {
		log.WithError(err).Warn("cannot read ServiceAccount token, skipping node labeling")
		return nil
	}

	caPool := x509.NewCertPool()
	if caData, err := os.ReadFile(caFile); err == nil {
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

	// JSON Merge Patch: add label iluvatar.ai/gpu=present.
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"iluvatar.ai/gpu": "present",
			},
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

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: caPool},
		},
	}
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

	log.WithField("node", nodeName).Info("node labeled with iluvatar.ai/gpu=present")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
