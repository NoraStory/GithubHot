"""healthz 与打分管道测试（工厂实例隔离，零环境变量操作）。

运行：python -m unittest discover -s tests -v
"""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from fastapi.testclient import TestClient  # noqa: E402

from app.main import create_app  # noqa: E402
from tests.helpers import payload  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
DUMMY = ROOT / "models" / "dummy.onnx"
if not DUMMY.exists():
    try:
        sys.path.insert(0, str(ROOT / "tools"))
        from make_dummy_onnx import build
        build(str(DUMMY))
    except Exception:
        raise unittest.SkipTest("无法构建 dummy 模型（缺 onnx 包），跳过本模块")

app = create_app(dev=True, model_path=str(DUMMY))


class Base(unittest.TestCase):
    """TestClient 必须进上下文才会触发 lifespan（加载引擎）。"""

    @classmethod
    def setUpClass(cls):
        cls.client = TestClient(app)
        cls._ctx = cls.client.__enter__()

    @classmethod
    def tearDownClass(cls):
        cls.client.__exit__(None, None, None)


class HealthTest(Base):
    def test_healthz_ok(self):
        r = self.client.get("/healthz")
        body = r.json()
        self.assertEqual(r.status_code, 200)
        self.assertEqual(body["status"], "ok")
        self.assertGreaterEqual(body["rss_mb"], 0)
        self.assertIn("p99_ms", body)
        self.assertIn("scored_24h", body)
        self.assertTrue(body["dev_mode"])


class ScorePipelineTest(Base):
    def test_score_roundtrip(self):
        r = self.client.post("/v1/score", json=payload(3))
        self.assertEqual(r.status_code, 200)
        body = r.json()
        self.assertEqual(set(body["scores"]), {"fp_0", "fp_1", "fp_2"})
        self.assertEqual(set(body["embeddings"]), body["scores"].keys())
        for s in body["scores"].values():
            self.assertTrue(0.0 <= s <= 1.0)
        self.assertTrue(body["elapsed_ms"] >= 0)

    def test_score_deterministic(self):
        a = self.client.post("/v1/score", json=payload(2, bias=0.5)).json()["scores"]
        b = self.client.post("/v1/score", json=payload(2, bias=0.5)).json()["scores"]
        self.assertEqual(a, b)

    def test_budget_rejected(self):
        self.assertEqual(self.client.post("/v1/score", json=payload(0)).status_code, 422)
        self.assertEqual(self.client.post("/v1/score", json=payload(101)).status_code, 422)

    def test_bad_edge_rejected(self):
        r = self.client.post("/v1/score", json=payload(2, edges=[[0, 5]]))
        self.assertEqual(r.status_code, 422)

    def test_bad_feature_dim_rejected(self):
        p = payload(1)
        p["nodes"][0]["features"] = p["nodes"][0]["features"][:-1]
        self.assertEqual(self.client.post("/v1/score", json=p).status_code, 422)

    def test_duplicate_id_rejected(self):
        p = payload(2)
        p["nodes"][1]["id"] = p["nodes"][0]["id"]
        self.assertEqual(self.client.post("/v1/score", json=p).status_code, 422)


if __name__ == "__main__":
    unittest.main()

