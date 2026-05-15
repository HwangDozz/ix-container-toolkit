// accelerator-dra-driver is a DRA (Dynamic Resource Allocation) driver
// for accelerator GPUs. It runs as a DaemonSet and:
//  1. Discovers GPU devices on the node using the active profile.
//  2. Publishes ResourceSlice objects to the API Server.
//  3. Implements NodePrepareResource/NodeUnprepareResource gRPC calls
//     from kubelet by generating CDI specs on-the-fly.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/device"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/dra"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

var (
	log = logrus.New()

	profilePath  string
	kubeconfig   string
	cdiDir       string
	nodeName     string
	resyncPeriod int
)

func main() {
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	flag.StringVar(&profilePath, "profile", "/etc/accelerator-toolkit/profiles/active.yaml",
		"Path to the accelerator profile YAML")
	flag.StringVar(&kubeconfig, "kubeconfig", "",
		"Path to kubeconfig (empty for in-cluster)")
	flag.StringVar(&cdiDir, "cdi-dir", "/etc/cdi",
		"Directory for CDI spec files")
	flag.StringVar(&nodeName, "node-name", "",
		"Node name (defaults to NODE_NAME env var)")
	flag.IntVar(&resyncPeriod, "resync-period", 60,
		"ResourceSlice resync period in seconds")
	flag.Parse()

	if nodeName == "" {
		nodeName = os.Getenv("NODE_NAME")
	}
	if nodeName == "" {
		log.Fatal("--node-name or NODE_NAME is required")
	}

	if err := run(); err != nil {
		log.WithError(err).Fatal("DRA driver failed")
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load profile.
	p, err := profile.Load(profilePath)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	log.WithField("profile", p.Metadata.Name).Info("Loaded profile")

	// Discover devices on this node.
	resolverCfg := device.ResolverConfigFromProfile(p)
	devs, err := device.DiscoverWithConfig("all", resolverCfg, log)
	if err != nil {
		return fmt.Errorf("discover devices: %w", err)
	}
	log.WithField("count", len(devs)).Info("Discovered devices")

	// Create Kubernetes client.
	k8sClient, err := buildK8sClient()
	if err != nil {
		return fmt.Errorf("build k8s client: %w", err)
	}

	// Create the DRA plugin.
	plugin := dra.NewPlugin(dra.PluginConfig{
		Profile: p,
		Devices: devs,
		CDIDir:  cdiDir,
	}, log)

	// Start the DRA helper (gRPC server + registration + ResourceSlice controller).
	helper, err := kubeletplugin.Start(ctx, plugin,
		kubeletplugin.DriverName(dra.DriverName),
		kubeletplugin.KubeClient(k8sClient),
		kubeletplugin.NodeName(nodeName),
		kubeletplugin.CDIDirectory(cdiDir),
	)
	if err != nil {
		return fmt.Errorf("start kubelet plugin: %w", err)
	}
	defer helper.Stop()

	// Publish discovered devices as ResourceSlices.
	resources := plugin.BuildPublishableResources(nodeName)
	if err := helper.PublishResources(ctx, resources); err != nil {
		return fmt.Errorf("publish resources: %w", err)
	}

	log.WithFields(logrus.Fields{
		"node":     nodeName,
		"driver":   dra.DriverName,
		"devices":  len(devs),
		"resource": p.Kubernetes.ResourceNames[0],
	}).Info("DRA driver started")

	// Block until context is cancelled.
	<-ctx.Done()
	log.Info("Shutting down")
	return nil
}

func buildK8sClient() (kubernetes.Interface, error) {
	var restConfig *rest.Config
	var err error

	if kubeconfig != "" {
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		restConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	return kubernetes.NewForConfig(restConfig)
}

func init() {
	// Ensure the profile directory exists for the default path.
	defaultDir := filepath.Dir("/etc/accelerator-toolkit/profiles/active.yaml")
	_ = os.MkdirAll(defaultDir, 0755)
}
