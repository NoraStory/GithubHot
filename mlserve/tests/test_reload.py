"""reload 热换语义测试：失败必须保留旧引擎（一次坏 reload 不打垮 /v1/score）。

运行：python -m unittest discover -s tests -v
"""
import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from fastapi.testclient import TestClient  # noqa: E402

from app import main as app_main  # noqa: E402
from app.main import create_app  # noqa: E402
from tests.helpers import payload  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
DUMMY = ROOT / "models" / "dummy.onnx"


class ReloadKeepOldEngineTest(unittest.TestCase):
    """坏模型 reload → 503 且 /v1/score 继续用旧引擎正常打分。"""

    @classmethod
    def setUpClass(cls):
        cls.app = create_app(dev=True, model_path=str(DUMMY))
        cls.client = TestClient(cls.app)
        cls._ctx = cls.client.__enter__()

    @classmethod
    def tearDownClass(cls):
        cls.client.__exit__(None, None, None)

    def test_bad_reload_keeps_old_engine(self):
        ok = self.client.post("/v1/score", json=payload(2))
        self.assertEqual(ok.status_code, 200)
        old_version = ok.json()["model_version"]

        with mock.patch.object(app_main, "load_engine", return_value=None):
            bad = self.client.post("/admin/reload")
            self.assertEqual(bad.status_code, 503)
            self.assertIn("已保留当前模型", bad.json()["detail"])

        # 旧引擎仍在位：打分不受影响，版本不变
        after = self.client.post("/v1/score", json=payload(2))
        self.assertEqual(after.status_code, 200)
        self.assertEqual(after.json()["model_version"], old_version)

    def test_good_reload_swaps_engine(self):
        r = self.client.post("/admin/reload")
        self.assertEqual(r.status_code, 200)
        self.assertTrue(r.json()["reloaded"])
        after = self.client.post("/v1/score", json=payload(2))
        self.assertEqual(after.json()["model_version"], r.json()["model_version"])
