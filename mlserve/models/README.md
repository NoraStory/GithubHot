# models/ — 模型文件目录（不入库）

| 文件 | 格式 | 来源 |
|---|---|---|
| `gnn_v1.onnx` | ONNX（推荐） | `scripts/ml/train_gnn.py` 导出（GraphSAGE 前向用纯张量算子，可被 onnxruntime 加载） |
| `gnn_v1.pt` | torch state_dict | ONNX 导出受阻时的回退；需含键 `l1_self / l1_neigh / l2_self / l2_neigh` |
| `dummy.onnx` | ONNX | `python tools/make_dummy_onnx.py` 生成，仅供管道联调 |

## 模型输入输出契约（ML_SERVE_FORMAT=onnx）

- 输入：`node_features` float32 `[N, F]`（F = ML_SERVE_FEATURE_DIM，默认 16，对齐 scripts/ml 的 16 个行为特征）；
  可选输入 `edge_index` int64 `[2, E]`。
- 输出（按顺序）：`scores` float32 `[N]`（**必须是概率 [0,1]**，模型内部完成 sigmoid）、
  `embeddings` float32 `[N, D]`（D = ML_SERVE_EMBED_DIM，默认 64）。
- 引擎按 session 实际声明的输入名喂参，未声明的输入自动省略。

## 热换模型

周训练产出新文件后（无需重启）：

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8790/admin/reload -Headers @{"X-Sidecar-Token"=$env:ML_SERVE_TOKEN}
```

Linux 下等价于规格书的 SIGHUP 语义（systemd 单元里已配置 ExecReload 调用此端点）。
