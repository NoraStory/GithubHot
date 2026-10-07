"""mlserve 配置——全部来自环境变量（对齐规格书 §9 P6-3b / §11.3）。"""
import os


def _get(key: str, default: str) -> str:
    v = os.getenv(key)
    return v if v not in (None, "") else default


HOST = _get("ML_SERVE_HOST", "127.0.0.1")          # 仅本机回环（安全红线）
PORT = int(_get("ML_SERVE_PORT", "8790"))
TOKEN = os.getenv("ML_SERVE_TOKEN", "")            # X-Sidecar-Token 共享密钥
DEV = _get("ML_SERVE_DEV", "0") == "1"             # 开发模式：无 token 时放行并告警
MODEL_PATH = _get("ML_SERVE_MODEL", os.path.join("models", "gnn_v1.onnx"))
FORMAT = _get("ML_SERVE_FORMAT", "auto")           # auto | onnx | torch | dummy
MAX_NODES = int(_get("ML_SERVE_MAX_NODES", "100"))  # 请求子图节点上限（规格书硬约束）
FEATURE_DIM = int(_get("ML_SERVE_FEATURE_DIM", "16"))   # 与 scripts/ml FEATURE_NAMES 对齐
EMBED_DIM = int(_get("ML_SERVE_EMBED_DIM", "64"))
SCORE_TIMEOUT_MS = int(_get("ML_SERVE_SCORE_TIMEOUT_MS", "500"))


def engine_format() -> str:
    """auto：按模型文件后缀推断；推断不出 → dummy。"""
    fmt = fmt_dynamic()
    p = model_path().lower()
    if fmt != "auto":
        return fmt
    if p.endswith(".onnx"):
        return "onnx"
    if p.endswith(".pt"):
        return "torch"
    return "dummy"


def model_path() -> str:
    """请求/加载时动态读取（测试可按用例切换模型）。"""
    return _get("ML_SERVE_MODEL", os.path.join(os.getcwd(), "models", "gnn_v1.onnx"))


def fmt_dynamic() -> str:
    return _get("ML_SERVE_FORMAT", "auto")


def token() -> str:
    """请求时动态读取（测试可按用例切换，无需重启进程）。"""
    return os.getenv("ML_SERVE_TOKEN", "")


def dev_mode() -> bool:
    return _get("ML_SERVE_DEV", "0") == "1"
