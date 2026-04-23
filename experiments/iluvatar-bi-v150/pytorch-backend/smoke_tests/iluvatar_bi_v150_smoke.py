#!/usr/bin/env python3
import argparse
import json
import os
import random
import sys


def print_json(name, value):
    print(f"## {name}", flush=True)
    print(json.dumps(value, ensure_ascii=False, indent=2), flush=True)


def import_torch():
    import torch

    try:
        import torch_npu  # noqa: F401
    except Exception:
        pass

    return torch


def resolve_device(torch):
    if hasattr(torch, "npu") and torch.npu.is_available():
        torch.npu.set_device(0)
        return torch.device("npu:0")
    if torch.cuda.is_available():
        torch.cuda.set_device(0)
        return torch.device("cuda:0")
    return torch.device("cpu")


def run_l3(torch, steps):
    device = resolve_device(torch)
    random.seed(20260423)
    torch.manual_seed(20260423)

    x = torch.randn(32, 128, device=device, requires_grad=True)
    w1 = torch.randn(128, 256, device=device, requires_grad=True)
    w2 = torch.randn(256, 10, device=device, requires_grad=True)
    optimizer = torch.optim.SGD([x, w1, w2], lr=1e-2)
    losses = []
    for _ in range(steps):
        optimizer.zero_grad(set_to_none=True)
        y = (x @ w1).relu() @ w2
        loss = y.pow(2).mean()
        loss.backward()
        optimizer.step()
        losses.append(float(loss.detach().cpu()))

    result = {
        "python": sys.version,
        "torch": getattr(torch, "__version__", ""),
        "torch_cuda_version": getattr(getattr(torch, "version", None), "cuda", None),
        "device": str(device),
        "cuda_is_available": bool(torch.cuda.is_available()),
        "cuda_device_count": int(torch.cuda.device_count()) if torch.cuda.is_available() else 0,
        "visible_devices": {
            "ILUVATAR_COREX_VISIBLE_DEVICES": os.environ.get("ILUVATAR_COREX_VISIBLE_DEVICES", ""),
            "ILUVATAR_COREX_REPLICA_DEVICES": os.environ.get("ILUVATAR_COREX_REPLICA_DEVICES", ""),
        },
        "steps": steps,
        "first_loss": losses[0],
        "final_loss": losses[-1],
    }
    print_json("iluvatar_bi_v150_smoke_l3", result)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--level", default="l3")
    parser.add_argument("--steps", type=int, default=10)
    args = parser.parse_args()

    if args.level != "l3":
        raise SystemExit(f"unsupported level: {args.level}")

    torch = import_torch()
    run_l3(torch, args.steps)


if __name__ == "__main__":
    main()
