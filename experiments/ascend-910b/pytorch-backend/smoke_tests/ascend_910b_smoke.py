#!/usr/bin/env python3
import argparse
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path


def run(cmd):
    proc = subprocess.run(cmd, text=True, capture_output=True, check=False)
    return {
        "cmd": cmd,
        "returncode": proc.returncode,
        "stdout": proc.stdout.strip(),
        "stderr": proc.stderr.strip(),
    }


def print_json(name, value):
    print(f"## {name}")
    print(json.dumps(value, ensure_ascii=False, indent=2))


def check_runtime_facts():
    env_keys = [
        "ASCEND_VISIBLE_DEVICES",
        "ASCEND_HOME_PATH",
        "ASCEND_TOOLKIT_HOME",
        "ASCEND_OPP_PATH",
        "ASCEND_AICPU_PATH",
        "LD_LIBRARY_PATH",
        "PYTHONPATH",
        "PATH",
    ]
    facts = {
        "env": {key: os.environ.get(key, "") for key in env_keys},
        "devices": sorted(str(path) for path in Path("/dev").glob("davinci*")),
        "control_devices": {
            path: Path(path).exists()
            for path in ["/dev/davinci_manager", "/dev/devmm_svm", "/dev/hisi_hdc"]
        },
        "ascend_paths": {
            path: Path(path).exists()
            for path in [
                "/usr/local/Ascend/driver/lib64/common",
                "/usr/local/Ascend/driver/lib64/driver",
                "/usr/local/Ascend/ascend-toolkit/latest/lib64",
                "/usr/local/Ascend/ascend-toolkit/latest/bin",
                "/usr/local/Ascend/ascend-toolkit/latest/tools/ccec_compiler/bin",
            ]
        },
        "npu_smi": shutil.which("npu-smi"),
    }
    print_json("runtime_facts", facts)

    if not facts["env"]["ASCEND_VISIBLE_DEVICES"]:
        raise RuntimeError("ASCEND_VISIBLE_DEVICES is empty")
    if not facts["devices"]:
        raise RuntimeError("no /dev/davinci* devices found")
    missing_controls = [path for path, exists in facts["control_devices"].items() if not exists]
    if missing_controls:
        raise RuntimeError(f"missing control devices: {missing_controls}")

    if facts["npu_smi"]:
        print_json("npu_smi_info", run([facts["npu_smi"], "info"]))


def check_torch_import():
    import torch

    try:
        import torch_npu  # noqa: F401
    except Exception as exc:
        raise RuntimeError(f"import torch_npu failed: {exc}") from exc

    info = {
        "python": sys.version,
        "torch": getattr(torch, "__version__", ""),
        "torch_npu_imported": True,
        "has_torch_npu_attr": hasattr(torch, "npu"),
    }
    if hasattr(torch, "npu"):
        info.update(
            {
                "npu_is_available": torch.npu.is_available(),
                "npu_device_count": torch.npu.device_count(),
                "npu_current_device": torch.npu.current_device()
                if torch.npu.is_available()
                else None,
            }
        )
    print_json("torch_import", info)

    if not info["has_torch_npu_attr"]:
        raise RuntimeError("torch.npu is unavailable after importing torch_npu")
    if not info.get("npu_is_available"):
        raise RuntimeError("torch.npu.is_available() is false")

    return torch


def check_tensor_ops(torch):
    device = torch.device("npu:0")
    x = torch.randn(256, 256, device=device, dtype=torch.float32, requires_grad=True)
    y = torch.randn(256, 256, device=device, dtype=torch.float32)
    z = x @ y
    loss = z.square().mean()
    loss.backward()
    value = float(loss.detach().cpu())
    print_json("tensor_ops", {"device": str(device), "loss": value})


def check_backward(torch):
    device = torch.device("npu:0")
    model = torch.nn.Sequential(
        torch.nn.Linear(32, 64),
        torch.nn.ReLU(),
        torch.nn.Linear(64, 8),
    ).to(device)
    x = torch.randn(16, 32, device=device)
    target = torch.randn(16, 8, device=device)
    optim = torch.optim.SGD(model.parameters(), lr=0.01)
    optim.zero_grad(set_to_none=True)
    loss = torch.nn.functional.mse_loss(model(x), target)
    loss.backward()
    optim.step()
    print_json("backward", {"loss": float(loss.detach().cpu())})


def check_train_steps(torch, steps):
    device = torch.device("npu:0")
    model = torch.nn.Sequential(
        torch.nn.Linear(128, 256),
        torch.nn.GELU(),
        torch.nn.Linear(256, 10),
    ).to(device)
    optim = torch.optim.AdamW(model.parameters(), lr=1e-3)
    losses = []
    for _ in range(steps):
        x = torch.randn(64, 128, device=device)
        target = torch.randint(0, 10, (64,), device=device)
        optim.zero_grad(set_to_none=True)
        loss = torch.nn.functional.cross_entropy(model(x), target)
        loss.backward()
        optim.step()
        losses.append(float(loss.detach().cpu()))
    print_json("train_steps", {"steps": steps, "losses": losses})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--level",
        choices=["l0", "l1", "l2", "l3"],
        default="l3",
        help="l0 runtime facts, l1 imports, l2 tensor/backward, l3 short train",
    )
    parser.add_argument("--steps", type=int, default=10)
    args = parser.parse_args()

    check_runtime_facts()
    if args.level == "l0":
        return
    torch = check_torch_import()
    if args.level == "l1":
        return
    check_tensor_ops(torch)
    check_backward(torch)
    if args.level == "l2":
        return
    check_train_steps(torch, args.steps)


if __name__ == "__main__":
    main()
