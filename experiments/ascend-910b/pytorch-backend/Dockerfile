ARG BASE_IMAGE=gpu-harbor.act.buaa.edu.cn/user-zhouxiao25/ascend/vllm-ascend:v0.11.0-crater-v0.0.2

FROM ${BASE_IMAGE}

LABEL org.opencontainers.image.title="ascend-910b-pytorch-backend"
LABEL org.opencontainers.image.description="Ascend 910B PyTorch backend image seed for toolkit runtime validation"

ENV PYTHONUNBUFFERED=1

WORKDIR /opt/ascend-910b-backend

COPY smoke_tests/ /opt/ascend-910b-backend/smoke_tests/

CMD ["python3", "/opt/ascend-910b-backend/smoke_tests/ascend_910b_smoke.py", "--level", "l3"]
