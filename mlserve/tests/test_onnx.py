"""ONNX 引擎测试：无 onnx/onnxruntime 时自动跳过；有则构建 dummy 模型并验证输出契约。"""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

ROOT = Path(__file__).resolve().parents[1]
DUMMY = ROOT / "models" / "dummy.onnx"

try:
    import onnx  # noqa: F401
    import onnxruntime  # noqa: F401
    HAS_ONNX = True
except ImportError:
    HAS_ONNX = False


@unittest.skipUnless(HAS_ONNX, "需要 onnx / onnxruntime")
class OnnxEngineTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not DUMMY.exists():
            sys.path.insert(0, str(ROOT / "tools"))
            from make_dummy_onnx import build
            build(str(DUMMY))

    def test_onnx_engine_contract(self):
        import numpy as np

        from app import engines
        eng = engines.load_engine("onnx", str(DUMMY))
        self.assertIsNotNone(eng)
        self.assertEqual(eng.format_name, "onnx")
        feats = np.tile(np.linspace(-1, 1, 16, dtype=np.float32), (4, 1))
        edges = np.array([[0, 1, 2], [1, 2, 3]], dtype=np.int64)
        scores, embeddings = eng.score(feats, edges)
        self.assertEqual(scores.shape, (4,))
        self.assertTrue(((scores >= 0) & (scores <= 1)).all())
        self.assertEqual(embeddings.shape, (4, 64))


if __name__ == "__main__":
    unittest.main()
