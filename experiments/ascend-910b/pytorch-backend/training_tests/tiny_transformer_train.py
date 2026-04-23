#!/usr/bin/env python3
import argparse
import json
import math
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
    except Exception as exc:
        raise RuntimeError(f"import torch_npu failed: {exc}") from exc

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


class TinyTransformerLM:
    def __init__(self, torch, vocab_size, seq_len, d_model, nhead, layers, dropout):
        nn = torch.nn
        self.torch = torch
        self.module = nn.Module()
        self.module.token = nn.Embedding(vocab_size, d_model)
        self.module.position = nn.Embedding(seq_len, d_model)
        encoder_layer = nn.TransformerEncoderLayer(
            d_model=d_model,
            nhead=nhead,
            dim_feedforward=d_model * 4,
            dropout=dropout,
            activation="gelu",
            batch_first=True,
            norm_first=True,
        )
        self.module.encoder = nn.TransformerEncoder(encoder_layer, num_layers=layers)
        self.module.norm = nn.LayerNorm(d_model)
        self.module.head = nn.Linear(d_model, vocab_size)
        scale = math.sqrt(d_model)

        def forward(input_ids):
            batch, length = input_ids.shape
            positions = torch.arange(length, device=input_ids.device).unsqueeze(0).expand(batch, length)
            x = self.module.token(input_ids) * scale
            x = x + self.module.position(positions)
            x = self.module.encoder(x)
            x = self.module.norm(x)
            return self.module.head(x)

        self.module.forward = forward


def synthetic_batch(torch, batch_size, seq_len, vocab_size, device):
    tokens = torch.randint(0, vocab_size, (batch_size, seq_len + 1), device=device)
    return tokens[:, :-1], tokens[:, 1:]


def train(args):
    torch = import_torch()
    local_rank = int(os.environ.get("LOCAL_RANK", "0"))
    device = resolve_device(torch, args.device, local_rank)
    dist = init_distributed(torch, device, args.dist_backend)

    seed = args.seed + dist.rank
    random.seed(seed)
    torch.manual_seed(seed)

    model = TinyTransformerLM(
        torch=torch,
        vocab_size=args.vocab_size,
        seq_len=args.seq_len,
        d_model=args.d_model,
        nhead=args.nhead,
        layers=args.layers,
        dropout=args.dropout,
    ).module.to(device)
    if dist.enabled:
        model = torch.nn.parallel.DistributedDataParallel(model, device_ids=[local_rank] if device.type != "cpu" else None)

    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr)
    criterion = torch.nn.CrossEntropyLoss()
    losses = []
    model.train()
    for _ in range(args.steps):
        x, y = synthetic_batch(torch, args.batch_size, args.seq_len, args.vocab_size, device)
        optimizer.zero_grad(set_to_none=True)
        logits = model(x)
        loss = criterion(logits.reshape(-1, args.vocab_size), y.reshape(-1))
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
        "seq_len": args.seq_len,
        "vocab_size": args.vocab_size,
        "d_model": args.d_model,
        "layers": args.layers,
        "first_loss": losses[0],
        "final_loss": losses[-1],
        "avg_final_loss": avg_final_loss,
    }
    if dist.rank == 0:
        print_json("tiny_transformer_train", result)

    if dist.enabled:
        torch.distributed.destroy_process_group()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--device", default="auto")
    parser.add_argument("--dist-backend", default="auto")
    parser.add_argument("--steps", type=int, default=20)
    parser.add_argument("--batch-size", type=int, default=8)
    parser.add_argument("--seq-len", type=int, default=64)
    parser.add_argument("--vocab-size", type=int, default=2048)
    parser.add_argument("--d-model", type=int, default=128)
    parser.add_argument("--nhead", type=int, default=4)
    parser.add_argument("--layers", type=int, default=2)
    parser.add_argument("--dropout", type=float, default=0.0)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--seed", type=int, default=20260423)
    args = parser.parse_args()
    train(args)


if __name__ == "__main__":
    main()
