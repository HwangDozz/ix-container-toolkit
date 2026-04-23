#!/usr/bin/env python3
import argparse
import json
import os
import random
import sys
from dataclasses import dataclass


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


def resolve_device(torch, requested, local_rank):
    if requested != "auto":
        return torch.device(requested)
    if hasattr(torch, "npu") and torch.npu.is_available():
        torch.npu.set_device(local_rank)
        return torch.device(f"npu:{local_rank}")
    if torch.cuda.is_available():
        torch.cuda.set_device(local_rank)
        return torch.device(f"cuda:{local_rank}")
    return torch.device("cpu")


def resolve_backend(device, requested):
    if requested != "auto":
        return requested
    if device.type == "npu":
        return "hccl"
    if device.type == "cuda":
        return "nccl"
    return "gloo"


class BasicBlock:
    expansion = 1

    def __init__(self, torch, in_planes, planes, stride=1):
        nn = torch.nn
        self.module = nn.Module()
        self.module.conv1 = nn.Conv2d(in_planes, planes, kernel_size=3, stride=stride, padding=1, bias=False)
        self.module.bn1 = nn.BatchNorm2d(planes)
        self.module.relu = nn.ReLU(inplace=True)
        self.module.conv2 = nn.Conv2d(planes, planes, kernel_size=3, stride=1, padding=1, bias=False)
        self.module.bn2 = nn.BatchNorm2d(planes)
        if stride != 1 or in_planes != planes:
            self.module.shortcut = nn.Sequential(
                nn.Conv2d(in_planes, planes, kernel_size=1, stride=stride, bias=False),
                nn.BatchNorm2d(planes),
            )
        else:
            self.module.shortcut = nn.Identity()

        def forward(x):
            out = self.module.relu(self.module.bn1(self.module.conv1(x)))
            out = self.module.bn2(self.module.conv2(out))
            out = out + self.module.shortcut(x)
            return self.module.relu(out)

        self.module.forward = forward


class TinyResNet:
    def __init__(self, torch, num_classes=10):
        nn = torch.nn
        self.in_planes = 32
        self.torch = torch
        self.module = nn.Module()
        self.module.stem = nn.Sequential(
            nn.Conv2d(3, 32, kernel_size=3, stride=1, padding=1, bias=False),
            nn.BatchNorm2d(32),
            nn.ReLU(inplace=True),
        )
        self.module.layer1 = self._make_layer(32, blocks=2, stride=1)
        self.module.layer2 = self._make_layer(64, blocks=2, stride=2)
        self.module.layer3 = self._make_layer(128, blocks=2, stride=2)
        self.module.pool = nn.AdaptiveAvgPool2d((1, 1))
        self.module.fc = nn.Linear(128, num_classes)

        def forward(x):
            out = self.module.stem(x)
            out = self.module.layer1(out)
            out = self.module.layer2(out)
            out = self.module.layer3(out)
            out = self.module.pool(out)
            out = out.flatten(1)
            return self.module.fc(out)

        self.module.forward = forward

    def _make_layer(self, planes, blocks, stride):
        layers = []
        layers.append(BasicBlock(self.torch, self.in_planes, planes, stride).module)
        self.in_planes = planes
        for _ in range(1, blocks):
            layers.append(BasicBlock(self.torch, self.in_planes, planes, 1).module)
        return self.torch.nn.Sequential(*layers)


@dataclass
class DistState:
    enabled: bool
    rank: int
    local_rank: int
    world_size: int
    backend: str


def init_distributed(torch, device, requested_backend):
    world_size = int(os.environ.get("WORLD_SIZE", "1"))
    rank = int(os.environ.get("RANK", "0"))
    local_rank = int(os.environ.get("LOCAL_RANK", "0"))
    if world_size <= 1:
        return DistState(False, rank, local_rank, world_size, "")

    backend = resolve_backend(device, requested_backend)
    torch.distributed.init_process_group(backend=backend, init_method="env://")
    return DistState(True, rank, local_rank, world_size, backend)


def train(args):
    torch = import_torch()
    local_rank = int(os.environ.get("LOCAL_RANK", "0"))
    device = resolve_device(torch, args.device, local_rank)
    dist = init_distributed(torch, device, args.dist_backend)

    seed = args.seed + dist.rank
    random.seed(seed)
    torch.manual_seed(seed)

    model = TinyResNet(torch, num_classes=args.classes).module.to(device)
    if dist.enabled:
        model = torch.nn.parallel.DistributedDataParallel(model, device_ids=[local_rank] if device.type != "cpu" else None)

    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr)
    criterion = torch.nn.CrossEntropyLoss()

    losses = []
    model.train()
    for step in range(args.steps):
        x = torch.randn(args.batch_size, 3, 32, 32, device=device)
        y = torch.randint(0, args.classes, (args.batch_size,), device=device)
        optimizer.zero_grad(set_to_none=True)
        loss = criterion(model(x), y)
        loss.backward()
        optimizer.step()
        losses.append(float(loss.detach().cpu()))

    if dist.enabled:
        loss_tensor = torch.tensor([losses[-1]], device=device)
        torch.distributed.all_reduce(loss_tensor, op=torch.distributed.ReduceOp.SUM)
        avg_final_loss = float((loss_tensor / dist.world_size).cpu()[0])
    else:
        avg_final_loss = losses[-1]

    result = {
        "python": sys.version,
        "torch": getattr(torch, "__version__", ""),
        "device": str(device),
        "distributed": dist.enabled,
        "backend": dist.backend,
        "rank": dist.rank,
        "local_rank": dist.local_rank,
        "world_size": dist.world_size,
        "steps": args.steps,
        "batch_size": args.batch_size,
        "first_loss": losses[0],
        "final_loss": losses[-1],
        "avg_final_loss": avg_final_loss,
    }
    if dist.rank == 0:
        print_json("portable_resnet_train", result)

    if dist.enabled:
        torch.distributed.destroy_process_group()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--device", default="auto")
    parser.add_argument("--dist-backend", default="auto")
    parser.add_argument("--steps", type=int, default=20)
    parser.add_argument("--batch-size", type=int, default=32)
    parser.add_argument("--classes", type=int, default=10)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--seed", type=int, default=20260423)
    args = parser.parse_args()
    train(args)


if __name__ == "__main__":
    main()
