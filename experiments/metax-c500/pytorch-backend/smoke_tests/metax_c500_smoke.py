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
    print(f"## {name}", flush=True)
    print(json.dumps(value, ensure_ascii=False, indent=2), flush=True)


def check_runtime_facts():
    env_keys = [
        "MACA_PATH",
        "MACA_CLANG_PATH",
        "MACA_INSATLL_PREFIX",
        "METAX_MXDRIVER_PREFER",
        "LD_LIBRARY_PATH",
        "LIBRARY_PATH",
        "PATH",
    ]
    facts = {
        "env": {key: os.environ.get(key, "") for key in env_keys},
        "devices": sorted(str(path) for path in Path("/dev").glob("mx*")),
        "dri_devices": sorted(str(path) for path in Path("/dev/dri").glob("*")) if Path("/dev/dri").exists() else [],
        "maca_paths": {
            path: Path(path).exists()
            for path in [
                "/opt/maca",
                "/opt/maca/lib",
                "/opt/maca/bin",
                "/opt/mxdriver",
                "/opt/mxdriver/lib",
                "/opt/mxdriver/bin",
            ]
        },
        "mx_smi": shutil.which("mx-smi"),
    }
    print_json("runtime_facts", facts)

    if "/dev/mxcd" not in facts["devices"]:
        raise RuntimeError("missing /dev/mxcd")
    if not facts["mx_smi"]:
        raise RuntimeError("mx-smi is not in PATH")

    print_json("mx_smi", run([facts["mx_smi"]]))


def check_torch_import():
    import torch

    info = {
        "python": sys.version,
        "torch": getattr(torch, "__version__", ""),
        "torch_cuda_version": getattr(getattr(torch, "version", None), "cuda", None),
        "cuda_is_available": torch.cuda.is_available(),
        "cuda_device_count": torch.cuda.device_count() if torch.cuda.is_available() else 0,
    }
    if info["cuda_is_available"]:
        info["cuda_device_name_0"] = torch.cuda.get_device_name(0)
    print_json("torch_import", info)

    if not info["cuda_is_available"]:
        raise RuntimeError("torch.cuda.is_available() is false")
    if info["cuda_device_count"] < 1:
        raise RuntimeError("torch.cuda.device_count() is less than 1")

    return torch


def check_tensor_ops(torch):
    device = torch.device("cuda:0")
    torch.cuda.set_device(0)
    x = torch.randn(256, 256, device=device, dtype=torch.float32, requires_grad=True)
    y = torch.randn(256, 256, device=device, dtype=torch.float32)
    z = x @ y
    loss = z.square().mean()
    loss.backward()
    torch.cuda.synchronize()
    print_json("tensor_ops", {"device": str(device), "loss": float(loss.detach().cpu())})


def check_backward(torch):
    device = torch.device("cuda:0")
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
    torch.cuda.synchronize()
    print_json("backward", {"loss": float(loss.detach().cpu())})


def check_train_steps(torch, steps):
    device = torch.device("cuda:0")
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
    torch.cuda.synchronize()
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
