# mlserve — GNN 在线推理 sidecar（规格书 §9 P6-3b）

独立端口 `127.0.0.1:8790`，内存受控（ONNX ≤ 200MB / torch 回退 ≤ 512MB）。
主服务经 HTTP 松耦合调用；本服务掉线不影响主服务（回落 Louvain 社区均值）。

模型文件 `models/gnn_v1.onnx` 为**单文件**（权重已内联，无外部 .data），
由 `..\scripts\ml\run_gnn_train.ps1` 训练后自动部署；`dummy.onnx` 仅管道联调用。

## 开发运行（Windows PowerShell）

```powershell
.\run_dev.ps1                 # 首次：建 .venv + 装依赖 + 生成 dummy 模型 + 起服务
# 验证：
Invoke-RestMethod http://127.0.0.1:8790/healthz
# 打分（dev 模式无需 token）：
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8790/v1/score `
  -ContentType "application/json" `
  -Body '{"nodes":[{"id":"fp_a","features":[0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]}],"edges":[]}'
```

## 测试

```powershell
.venv\Scripts\python -m unittest discover -s tests -v
```

- `test_pipeline`：healthz 三态字段、打分往返、确定性、子图预算（≤100 节点）、边/特征维度校验（dummy 引擎，零模型依赖）
- `test_auth`：token 缺失/错误 403、正确放行、reload 鉴权、healthz 免鉴权
- `test_onnx`：装了 onnx/onnxruntime 时自动构建 dummy 模型并验证 [N] + [N,64] 输出契约；未装自动跳过

## 真模型验证（2026-10-05，GraphSAGE gnn_v1）

- `/healthz`：`status=ok`，`model_version=onnx:...\models\gnn_v1.onnx`，RSS **82.2MB**（红线 200MB）
- `/v1/score` 语义检验：机械击键型 bot 特征 **0.982**，人类特征 **0.0**（经共享零特征 ip 连接子消息传递）
- `/admin/reload`：热换成功（load 0.003s）
- go-client 真机冒烟：`SIDECAR_LIVE_URL=http://127.0.0.1:8790 go test -run TestLiveScore -v` PASS

## 内存红线（硬约束，验收见规格书 §13.2）

| 项 | 值 |
|---|---|
| ML_SERVE_HOST | 仅 127.0.0.1（禁止 0.0.0.0） |
| worker 数 | 1（代码内固定，无 --workers） |
| 线程 | OMP_NUM_THREADS=1 + 单线程 session |
| 子图 | ≤ 100 节点（ML_SERVE_MAX_NODES），批量恒为 1 |
| systemd | MemoryMax=320M（ONNX）/ 560M（torch 回退），OOM 自动重启 |
| RSS 目标 | ONNX ≤ 200MB；torch 回退 ≤ 512MB |

## Linux 部署

```bash
sudo cp deploy/githubhot-gnn.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now githubhot-gnn
sudo systemctl edit githubhot-gnn     # ML_SERVE_TOKEN=... 写入 override.conf
```

主服务侧配置（规格书 §11.3）：`GNN_SIDECAR_URL=http://127.0.0.1:8790` + `GNN_SIDECAR_TOKEN=<同值>`。
