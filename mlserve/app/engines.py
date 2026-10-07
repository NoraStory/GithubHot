"""推理引擎三态：onnx（首选，低内存）→ torch（导出受阻回退）→ dummy（管道联调）。

约定（对齐规格书 §9 P6-3b）：
- 引擎输入：node_features [N, F] float32 + edge_index [2, E] int64（onnx 模型可只声明 node_features）
- 引擎输出：scores [N]（已是概率 [0,1]）+ embeddings [N, D]
- 子图规模：N ≤ MAX_NODES（config），批量恒为 1
"""
import math
import time

import numpy as np

from app import config


class EngineError(Exception):
    pass


class BaseEngine:
    format_name = "base"
    version = "none"

    def score(self, feats: np.ndarray, edges: np.ndarray):
        raise NotImplementedError


class DummyEngine(BaseEngine):
    """无模型时的联调引擎：确定性伪分数（仅供管道验证，非真实 GNN）。"""

    format_name = "dummy"
    version = "dummy-v0"

    def score(self, feats: np.ndarray, edges: np.ndarray):
        mean = feats.mean(axis=1)
        scores = 1.0 / (1.0 + np.exp(-(mean * 3.0)))
        # 确定性伪嵌入：由均值/方差/极值拼出 EMBED_DIM 维
        basis = np.concatenate([feats.mean(axis=0), feats.std(axis=0),
                                feats.min(axis=0), feats.max(axis=0)])
        tile = int(math.ceil(config.EMBED_DIM / max(1, basis.size)))
        emb = np.tile(basis, tile)[: config.EMBED_DIM]
        embeddings = np.repeat(emb[None, :], feats.shape[0], axis=0)
        return scores.astype(np.float32), embeddings.astype(np.float32)


class OnnxEngine(BaseEngine):
    format_name = "onnx"

    def __init__(self, path: str):
        import onnxruntime as ort

        so = ort.SessionOptions()
        so.intra_op_num_threads = 1
        so.inter_op_num_threads = 1
        self.sess = ort.InferenceSession(path, sess_options=so,
                                         providers=["CPUExecutionProvider"])
        self.version = f"onnx:{path}"
        self.in_names = [i.name for i in self.sess.get_inputs()]

    def score(self, feats: np.ndarray, edges: np.ndarray):
        feed = {}
        if "node_features" in self.in_names:
            feed["node_features"] = feats.astype(np.float32)
        if "edge_index" in self.in_names:
            feed["edge_index"] = edges.astype(np.int64)
        outs = self.sess.run(None, feed)
        scores = np.asarray(outs[0]).reshape(-1)
        embeddings = np.asarray(outs[1]) if len(outs) > 1 else np.zeros(
            (feats.shape[0], config.EMBED_DIM), dtype=np.float32)
        return scores.astype(np.float32), embeddings.astype(np.float32)


class TorchEngine(BaseEngine):
    """torch 回退模式：2 层 GraphSAGE（mean 聚合），权重来自 state_dict。"""

    format_name = "torch"

    def __init__(self, path: str):
        import torch

        torch.set_num_threads(1)
        self.torch = torch
        sd = torch.load(path, map_location="cpu", weights_only=True)
        need = {"l1_self", "l1_neigh", "l2_self", "l2_neigh"}
        missing = need - set(sd)
        if missing:
            raise EngineError(f"torch 模型缺少权重键: {sorted(missing)}")
        self.sd = {k: v.float() for k, v in sd.items()}
        self.version = f"torch:{path}"

    def score(self, feats: np.ndarray, edges: np.ndarray):
        t = self.torch
        with t.inference_mode():
            x = t.from_numpy(feats.astype(np.float32))
            src = t.from_numpy(edges[0].astype(np.int64))
            dst = t.from_numpy(edges[1].astype(np.int64))
            if x.shape[0] > 0 and src.numel() > 0:
                agg = t.zeros_like(x).index_add_(0, dst, x[src])
                deg = t.zeros(x.shape[0]).index_add_(0, dst, t.ones(src.numel()))
                agg = agg / deg.clamp(min=1.0).unsqueeze(1)
            else:
                agg = t.zeros_like(x)
            h = t.relu(x @ self.sd["l1_self"].t() + agg @ self.sd["l1_neigh"].t())
            emb = t.relu(h @ self.sd["l2_self"].t() + agg @ self.sd["l2_neigh"].t())
            logits = (emb @ self.sd["l2_self"].t()).sum(dim=1)
            scores = t.sigmoid(logits)
        return scores.numpy().astype(np.float32), emb.numpy().astype(np.float32)


def load_engine(fmt: str | None = None, path: str | None = None):
    """按格式加载引擎；显式参数优先，否则读动态配置。模型缺失且要求 onnx/torch → None（带失败原因日志）。"""
    if fmt in (None, "", "auto"):
        fmt = config.engine_format()  # 解析 auto → onnx/torch/dummy
    path = path or config.model_path()
    t0 = time.time()
    try:
        if fmt == "onnx":
            eng = OnnxEngine(path)
        elif fmt == "torch":
            eng = TorchEngine(path)
        else:
            eng = DummyEngine()
    except Exception as e:  # noqa: BLE001 - 加载失败原因必须可见，否则排障只剩 "degraded"
        import traceback
        print(f"[mlserve] 模型加载失败 format={fmt} path={path}: {e}\n{traceback.format_exc()}")
        return None
    eng.load_seconds = round(time.time() - t0, 3)
    return eng
