package profile

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	renderedAppName         = "accelerator-toolkit"
	renderedNamespace       = "kube-system"
	renderedServiceAccount  = "accelerator-toolkit"
	renderedPauseName       = "accelerator-toolkit-pause"
	renderedHostConfigDir   = "/etc/accelerator-toolkit"
	renderedProfileEnvName  = "ACCELERATOR_PROFILE_FILE"
	renderedLogLevelEnvName = "ACCELERATOR_LOG_LEVEL"
)

type RuntimeClassManifest struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   RuntimeClassMetadata   `yaml:"metadata"`
	Handler    string                 `yaml:"handler"`
	Scheduling RuntimeClassScheduling `yaml:"scheduling,omitempty"`
}

type RuntimeClassMetadata struct {
	Name string `yaml:"name"`
}

type DaemonSetManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   ObjectMeta    `yaml:"metadata"`
	Spec       DaemonSetSpec `yaml:"spec"`
}

type ServiceAccountManifest struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   ObjectMeta `yaml:"metadata"`
}

type ClusterRoleManifest struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   ObjectMeta   `yaml:"metadata"`
	Rules      []PolicyRule `yaml:"rules"`
}

type PolicyRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

type ClusterRoleBindingManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   ObjectMeta    `yaml:"metadata"`
	RoleRef    RoleRef       `yaml:"roleRef"`
	Subjects   []RBACSubject `yaml:"subjects"`
}

type RoleRef struct {
	APIGroup string `yaml:"apiGroup"`
	Kind     string `yaml:"kind"`
	Name     string `yaml:"name"`
}

type RBACSubject struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type ObjectMeta struct {
	Name      string            `yaml:"name,omitempty"`
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type DaemonSetSpec struct {
	Selector       LabelSelector   `yaml:"selector"`
	UpdateStrategy UpdateStrategy  `yaml:"updateStrategy"`
	Template       PodTemplateSpec `yaml:"template"`
}

type LabelSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels"`
}

type UpdateStrategy struct {
	Type string `yaml:"type"`
}

type PodTemplateSpec struct {
	Metadata ObjectMeta `yaml:"metadata"`
	Spec     PodSpec    `yaml:"spec"`
}

type PodSpec struct {
	ServiceAccountName string            `yaml:"serviceAccountName"`
	NodeSelector       map[string]string `yaml:"nodeSelector,omitempty"`
	Tolerations        []Toleration      `yaml:"tolerations,omitempty"`
	HostPID            bool              `yaml:"hostPID,omitempty"`
	InitContainers     []Container       `yaml:"initContainers,omitempty"`
	Containers         []Container       `yaml:"containers"`
	Volumes            []Volume          `yaml:"volumes,omitempty"`
}

type Container struct {
	Name            string            `yaml:"name"`
	Image           string            `yaml:"image"`
	ImagePullPolicy string            `yaml:"imagePullPolicy,omitempty"`
	Command         []string          `yaml:"command,omitempty"`
	SecurityContext *SecurityContext  `yaml:"securityContext,omitempty"`
	Env             []EnvVar          `yaml:"env,omitempty"`
	VolumeMounts    []VolumeMount     `yaml:"volumeMounts,omitempty"`
	Resources       *ResourceRequests `yaml:"resources,omitempty"`
}

type SecurityContext struct {
	Privileged bool `yaml:"privileged,omitempty"`
}

type EnvVar struct {
	Name      string        `yaml:"name"`
	Value     string        `yaml:"value,omitempty"`
	ValueFrom *EnvValueFrom `yaml:"valueFrom,omitempty"`
}

type EnvValueFrom struct {
	FieldRef *ObjectFieldSelector `yaml:"fieldRef,omitempty"`
}

type ObjectFieldSelector struct {
	FieldPath string `yaml:"fieldPath"`
}

type VolumeMount struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
}

type ResourceRequests struct {
	Requests map[string]string `yaml:"requests,omitempty"`
}

type Volume struct {
	Name     string    `yaml:"name"`
	HostPath *HostPath `yaml:"hostPath,omitempty"`
}

type HostPath struct {
	Path string `yaml:"path"`
}

// RenderRuntimeClassYAML renders a RuntimeClass manifest from the profile.
func (p *Profile) RenderRuntimeClassYAML() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate profile before rendering runtimeclass: %w", err)
	}

	manifest := RuntimeClassManifest{
		APIVersion: "node.k8s.io/v1",
		Kind:       "RuntimeClass",
		Metadata: RuntimeClassMetadata{
			Name: p.Runtime.RuntimeClassName,
		},
		Handler:    p.Runtime.HandlerName,
		Scheduling: p.Kubernetes.RuntimeClassScheduling,
	}

	data, err := yaml.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal runtimeclass manifest: %w", err)
	}
	return data, nil
}

// RenderDaemonSetYAML renders the installer DaemonSet manifest from the profile.
func (p *Profile) RenderDaemonSetYAML(image, sourceProfilePath string) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate profile before rendering daemonset: %w", err)
	}
	if image == "" {
		return nil, fmt.Errorf("image is required")
	}

	profileInImage := "/profiles/" + filepath.Base(sourceProfilePath)
	if sourceProfilePath == "" {
		profileInImage = "/profiles/" + p.Metadata.Name + ".yaml"
	}

	labels := map[string]string{"app": renderedAppName}
	nodeSelector := p.Kubernetes.NodeLabels
	if len(p.Kubernetes.RuntimeClassScheduling.NodeSelector) > 0 {
		nodeSelector = p.Kubernetes.RuntimeClassScheduling.NodeSelector
	}
	tolerations := []Toleration{
		{Operator: "Exists", Effect: "NoSchedule"},
		{Operator: "Exists", Effect: "NoExecute"},
	}
	if len(p.Kubernetes.RuntimeClassScheduling.Tolerations) > 0 {
		tolerations = p.Kubernetes.RuntimeClassScheduling.Tolerations
	}

	manifest := DaemonSetManifest{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Metadata: ObjectMeta{
			Name:      renderedAppName,
			Namespace: renderedNamespace,
			Labels:    labels,
		},
		Spec: DaemonSetSpec{
			Selector: LabelSelector{MatchLabels: labels},
			UpdateStrategy: UpdateStrategy{
				Type: "RollingUpdate",
			},
			Template: PodTemplateSpec{
				Metadata: ObjectMeta{Labels: labels},
				Spec: PodSpec{
					ServiceAccountName: renderedServiceAccount,
					NodeSelector:       nodeSelector,
					Tolerations:        tolerations,
					HostPID:            true,
					InitContainers: []Container{
						{
							Name:            "accelerator-installer",
							Image:           image,
							ImagePullPolicy: "IfNotPresent",
							SecurityContext: &SecurityContext{Privileged: true},
							Env: []EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &EnvValueFrom{
										FieldRef: &ObjectFieldSelector{FieldPath: "spec.nodeName"},
									},
								},
								{Name: "HOST_BIN_DIR", Value: "/usr/local/bin"},
								{Name: "HOST_CONFIG_DIR", Value: renderedHostConfigDir},
								{Name: "HOST_MOUNT", Value: "/host"},
								{Name: renderedProfileEnvName, Value: profileInImage},
								{Name: "RESTART_CONTAINERD", Value: "true"},
								{Name: renderedLogLevelEnvName, Value: "info"},
							},
							VolumeMounts: []VolumeMount{
								{Name: "host-root", MountPath: "/host"},
								{Name: "host-run", MountPath: "/host/run"},
							},
						},
					},
					Containers: []Container{
						{
							Name:  renderedPauseName,
							Image: "registry.k8s.io/pause:3.9",
							Resources: &ResourceRequests{
								Requests: map[string]string{
									"cpu":    "10m",
									"memory": "10Mi",
								},
							},
						},
					},
					Volumes: []Volume{
						{Name: "host-root", HostPath: &HostPath{Path: "/"}},
						{Name: "host-run", HostPath: &HostPath{Path: "/run"}},
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(&manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal daemonset manifest: %w", err)
	}
	return data, nil
}

// RenderRBACYAML renders the static RBAC needed by the installer DaemonSet.
func (p *Profile) RenderRBACYAML() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validate profile before rendering rbac: %w", err)
	}

	serviceAccount := ServiceAccountManifest{
		APIVersion: "v1",
		Kind:       "ServiceAccount",
		Metadata: ObjectMeta{
			Name:      renderedServiceAccount,
			Namespace: renderedNamespace,
		},
	}
	clusterRole := ClusterRoleManifest{
		APIVersion: "rbac.authorization.k8s.io/v1",
		Kind:       "ClusterRole",
		Metadata: ObjectMeta{
			Name: renderedAppName,
		},
		Rules: []PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "patch", "update"},
			},
		},
	}
	clusterRoleBinding := ClusterRoleBindingManifest{
		APIVersion: "rbac.authorization.k8s.io/v1",
		Kind:       "ClusterRoleBinding",
		Metadata: ObjectMeta{
			Name: renderedAppName,
		},
		RoleRef: RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     renderedAppName,
		},
		Subjects: []RBACSubject{
			{
				Kind:      "ServiceAccount",
				Name:      renderedServiceAccount,
				Namespace: renderedNamespace,
			},
		},
	}

	serviceAccountYAML, err := yaml.Marshal(&serviceAccount)
	if err != nil {
		return nil, fmt.Errorf("marshal serviceaccount: %w", err)
	}
	clusterRoleYAML, err := yaml.Marshal(&clusterRole)
	if err != nil {
		return nil, fmt.Errorf("marshal clusterrole: %w", err)
	}
	clusterRoleBindingYAML, err := yaml.Marshal(&clusterRoleBinding)
	if err != nil {
		return nil, fmt.Errorf("marshal clusterrolebinding: %w", err)
	}

	return []byte("---\n" + string(serviceAccountYAML) + "---\n" + string(clusterRoleYAML) + "---\n" + string(clusterRoleBindingYAML)), nil
}

// RenderBundleYAML renders the deploy bundle: RBAC + RuntimeClass + DaemonSet.
func (p *Profile) RenderBundleYAML(image, sourceProfilePath string) ([]byte, error) {
	rbacYAML, err := p.RenderRBACYAML()
	if err != nil {
		return nil, err
	}
	runtimeClassYAML, err := p.RenderRuntimeClassYAML()
	if err != nil {
		return nil, err
	}
	daemonSetYAML, err := p.RenderDaemonSetYAML(image, sourceProfilePath)
	if err != nil {
		return nil, err
	}

	return []byte(string(rbacYAML) + "---\n" + string(runtimeClassYAML) + "---\n" + string(daemonSetYAML)), nil
}
