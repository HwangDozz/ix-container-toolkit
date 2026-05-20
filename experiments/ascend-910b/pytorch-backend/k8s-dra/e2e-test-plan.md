# Ascend 910B DRA + CDI E2E Test Plan

## Goal

Validate that a Pod requesting an Ascend 910B through Kubernetes DRA receives only the allocated device, required Ascend control devices, CDI-provided environment variables, and driver/toolkit mounts.

## Preconditions

- Kubernetes v1.35+ with `resource.k8s.io/v1` DRA APIs.
- Ascend node `kunlun-02` is Ready and has `/dev/davinci0..7`.
- containerd has CDI enabled and reads `/etc/cdi`.
- RuntimeClass `runc-cdi` points to plain runc, not `ascend-docker-runtime`.
- DRA driver is running on `kunlun-02` and publishes `gpu.accelerator-toolkit.io` ResourceSlices.
- Namespace `crater-workspace` exists.

## Test Sequence

1. Apply the DeviceClass and ResourceClaimTemplate.
2. Confirm ResourceSlices exist for `gpu.accelerator-toolkit.io` on `kunlun-02`.
3. Run `debug-pod.yaml` and verify CDI injection basics:
   - exactly one `/dev/davinciN` device is present;
   - `/dev/davinci_manager`, `/dev/devmm_svm`, and `/dev/hisi_hdc` are present;
   - `ASCEND_VISIBLE_DEVICES=all` and Ascend toolkit env vars are present;
   - `/etc/cdi/huawei.json` contains a device entry with the claim UID suffix.
4. Delete the debug Pod and verify the claim-specific CDI entry is removed.
5. Run `smoke-job.yaml` for L3 runtime smoke.
6. Run `train-single-job.yaml` for a short ResNet training check.
7. Run `train-transformer-job.yaml` for a short transformer training check.

## Commands

```bash
kubectl apply -f deviceclass.yaml
kubectl apply -f resourceclaim-template.yaml
kubectl get resourceslices -o wide | grep gpu.accelerator-toolkit.io

kubectl apply -f debug-pod.yaml
kubectl -n crater-workspace wait --for=condition=Ready pod/dra-cdi-debug --timeout=180s || true
kubectl -n crater-workspace logs pod/dra-cdi-debug
kubectl -n crater-workspace delete pod dra-cdi-debug

kubectl apply -f smoke-job.yaml
kubectl -n crater-workspace wait --for=condition=Complete job/dra-cdi-smoke --timeout=600s
kubectl -n crater-workspace logs job/dra-cdi-smoke

kubectl apply -f train-single-job.yaml
kubectl -n crater-workspace wait --for=condition=Complete job/dra-cdi-train-single --timeout=900s
kubectl -n crater-workspace logs job/dra-cdi-train-single

kubectl apply -f train-transformer-job.yaml
kubectl -n crater-workspace wait --for=condition=Complete job/dra-cdi-train-transformer --timeout=900s
kubectl -n crater-workspace logs job/dra-cdi-train-transformer
```

## Pass Criteria

- ResourceClaim becomes allocated and reserved.
- Pod starts with `runtimeClassName: runc-cdi`.
- Container sees only the allocated `/dev/davinciN` plus control devices.
- Smoke and short training jobs complete successfully.
- Deleting Pods/Jobs removes claim-specific CDI entries from `/etc/cdi/huawei.json`.

## Current Run Notes

Run date: 2026-05-20.

Validated:

- `DeviceClass` and `ResourceClaimTemplate` apply successfully.
- `ResourceSlice` exists for driver `gpu.accelerator-toolkit.io`, pool `kunlun-02`, devices `index-0..index-7`.
- `ResourceClaim` reaches `allocated,reserved` and DRA driver logs `Prepared resource claim` with a claim-scoped CDI device ID.
- CDI cleanup works: after the failed smoke Job, `/etc/cdi/huawei.json` was removed by `UnprepareResourceClaims`.
- The original `python3 not found` blocker is handled by resolving `python3`, `python`, and `/usr/local/python*/bin/*`, with an `apt-get install python3` fallback.
- The `libhccl.so: file too short` blocker was traced to zero-byte host shared libraries. CDI generation now skips zero-byte `.so` files, preserving valid image libraries.
- Re-running smoke confirmed `/usr/local/Ascend/cann-8.5.1/lib64/libhccl.so` is a valid 642K ELF and `torch_npu` imports successfully.

Required driver deployment shape:

- Run the DRA driver as a Kubernetes Pod/DaemonSet with the `accelerator-dra-driver` ServiceAccount so it uses in-cluster config.
- Mount host `/etc/cdi` at the CDI output path used by the driver.
- Mount host `/usr/local/Ascend` into the driver container at the same `/usr/local/Ascend` path; otherwise optional hostPaths are incorrectly skipped during CDI generation.
- Avoid hand-starting the driver with `/home/huangsy/.kube/config` on `kunlun-02`; that path produced `dial tcp 192.168.5.60:6443: connect: network is unreachable` during `NodePrepareResources`.

Current blockers:

- Smoke now reaches application runtime but fails because `torch.npu.is_available()` is false and `torch.npu.device_count()` is 0.
- In the smoke output before fixing the driver Pod mount shape, `driver/lib64/common` and `driver/lib64/driver` were absent from the container. The next run must use a DRA driver Pod that can see host `/usr/local/Ascend` at the same path before generating CDI.
- `kunlun-02` also shows intermittent kubelet/containerd `context deadline exceeded` while starting ordinary containers, including the DRA driver Pod and diagnostic Pod. This currently prevents completing the final smoke/train rerun reliably.

Training status:

- `train-single-job.yaml` and `train-transformer-job.yaml` were not run after the smoke failure because both depend on the same `torch.npu.is_available()` prerequisite.
