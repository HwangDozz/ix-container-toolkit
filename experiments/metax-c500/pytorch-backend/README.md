# Metax C500 PyTorch Backend Validation

This directory validates the Metax C500 MACA PyTorch backend image on Kubernetes.

## Baseline

- Node: `greatwall-02`
- RuntimeClass: `metax`
- Image: `cr.metax-tech.com/public-library/maca-pytorch:3.5.3.6-torch2.4-py310-kylinv10-arm64`
- Resource shape: `metax-tech.com/gpu: 2`, `cpu: 16`, `memory: 128Gi`

The jobs intentionally request 2 GPUs. A 1-GPU resource shape exposed a MACA PyTorch device enumeration assertion during initial validation.

## ConfigMaps

```bash
kubectl create configmap metax-c500-smoke-tests \
  -n crater-workspace \
  --from-file=metax_c500_smoke.py=experiments/metax-c500/pytorch-backend/smoke_tests/metax_c500_smoke.py \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create configmap metax-c500-portable-training-tests \
  -n crater-workspace \
  --from-file=portable_resnet_train.py=experiments/ascend-910b/pytorch-backend/training_tests/portable_resnet_train.py \
  --from-file=tiny_transformer_train.py=experiments/ascend-910b/pytorch-backend/training_tests/tiny_transformer_train.py \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Jobs

```bash
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/smoke-job.yaml
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/train-single-job.yaml
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/train-transformer-job.yaml
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/train-ddp-2card-job.yaml
```

After deploying `profiles/metax-c500.yaml` through this project's installer, use the xpu-runtime smoke job:

```bash
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/xpu-runtime-smoke-job.yaml
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/xpu-runtime-train-ddp-2card-job.yaml
```

The xpu-runtime smoke job explicitly sets `METAX_VISIBLE_DEVICES=all` because the current runtime shim only injects the hook when a profile selector env is already present in the OCI spec.

## xpu-runtime Installer

The validated installer image is:

```text
crater-harbor.act.buaa.edu.cn/xpu-huangsy/accelerator-toolkit-installer:metax-c500-20260424
```

Use the one-shot installer job to avoid replacing the existing Ascend DaemonSet:

```bash
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/xpu-runtime-installer-job.yaml
```

If containerd has not loaded the `xpu-runtime` handler, restart containerd on `greatwall-02`:

```bash
kubectl apply -f experiments/metax-c500/pytorch-backend/k8s/containerd-restart-job.yaml
```

## Passing Jobs

- `crater-workspace/metax-c500-backend-smoke-v2`
- `crater-workspace/metax-c500-portable-train-single`
- `crater-workspace/metax-c500-portable-train-transformer`
- `crater-workspace/metax-c500-portable-train-ddp-2card`
- `crater-workspace/metax-c500-xpu-runtime-smoke`
- `crater-workspace/metax-c500-xpu-runtime-train-ddp-2card`
