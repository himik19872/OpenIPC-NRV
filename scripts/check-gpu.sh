#!/usr/bin/env bash
# Проверка, что GPU NVIDIA доступна внутри Docker-контейнера.
# Запускать на хосте: bash scripts/check-gpu.sh
set -uo pipefail

echo "════════════════════════════════════════════════"
echo " 1. Драйвер на хосте"
echo "════════════════════════════════════════════════"
if command -v nvidia-smi >/dev/null 2>&1; then
  nvidia-smi --query-gpu=name,driver_version,memory.total,compute_cap \
             --format=csv,noheader 2>&1
else
  echo "  ✗ nvidia-smi не найден — драйвер не установлен"
  exit 1
fi

echo
echo "════════════════════════════════════════════════"
echo " 2. Docker runtime"
echo "════════════════════════════════════════════════"
docker info 2>/dev/null | grep -iE "default runtime|runtime:" | head -3
echo "  --- CDI устройства ---"
docker info 2>/dev/null | grep -i "cdi:" | head -5

echo
echo "════════════════════════════════════════════════"
echo " 3. GPU внутри контейнера (CUDA runtime)"
echo "════════════════════════════════════════════════"
docker run --rm --gpus all nvidia/cuda:12.6.2-base-ubuntu24.04 nvidia-smi \
  --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>&1

echo
echo "════════════════════════════════════════════════"
echo " 4. Проверка torch + CUDA (в образе ai-detector)"
echo "════════════════════════════════════════════════"
if docker image inspect gigacode-ai-detector >/dev/null 2>&1; then
  docker run --rm --gpus all --entrypoint python gigacode-ai-detector -c "
import torch
print('torch            :', torch.__version__)
print('cuda build       :', torch.version.cuda)
print('cuda available   :', torch.cuda.is_available())
if torch.cuda.is_available():
    print('device           :', torch.cuda.get_device_name(0))
    print('compute cap      :', 'sm_%d%d' % torch.cuda.get_device_capability(0))
    print('VRAM             :', round(torch.cuda.get_device_properties(0).total_memory/1024**3, 1), 'GiB')
    a = torch.rand(1000, 1000, device='cuda')
    print('matmul on GPU    : OK', (a @ a).sum().item() != 0)
" 2>&1
else
  echo "  ⓘ образ gigacode-ai-detector не собран — пропускаю"
  echo "    соберите: docker compose build ai-detector"
fi
