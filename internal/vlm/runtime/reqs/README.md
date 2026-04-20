# Hash-pinned requirements

Generated via `uv pip compile --generate-hashes`. Regenerate when bumping mlx_vlm or vllm-mlx.

## mlx_vlm

```bash
echo 'mlx-vlm==0.4.4' > /tmp/in.txt
uv pip compile /tmp/in.txt --generate-hashes \
  --output-file internal/vlm/runtime/reqs/requirements_mlx_vlm.txt \
  --python-version 3.12
```

## vllm-mlx

```bash
echo 'vllm-mlx' > /tmp/in.txt
uv pip compile /tmp/in.txt --generate-hashes \
  --output-file internal/vlm/runtime/reqs/requirements_vllm_mlx.txt \
  --python-version 3.12
```

After regenerating, run `make test` and the `integration` MLX runner to verify the new pin set installs cleanly.
