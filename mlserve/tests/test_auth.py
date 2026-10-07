"""token 鉴权与热换模型测试（工厂实例，零环境变量操作）。"""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from fastapi.testclient import TestClient  # noqa: E402

from app.main import create_app  # noqa: E402
from tests.helpers import payload  # noqa: E402

TOKEN = "test-token-12345"
DUMMY = Path(__file__).resolve().parents[1] / "models" / "dummy.onnx"

app = create_app(token=TOKEN, dev=False, model_path=str(DUMMY))


class Base(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.client = TestClient(app)
        cls._ctx = cls.client.__enter__()  # 触发 lifespan（加载引擎）

    @classmethod
    def tearDownClass(cls):
        cls.client.__exit__(None, None, None)


class AuthTest(Base):
    def test_missing_token_rejected(self):
        self.assertEqual(self.client.post("/v1/score", json=payload(1)).status_code, 403)

    def test_wrong_token_rejected(self):
        r = self.client.post("/v1/score", json=payload(1), headers={"X-Sidecar-Token": "nope"})
        self.assertEqual(r.status_code, 403)

    def test_healthz_no_auth_needed(self):
        r = self.client.get("/healthz")
        self.assertEqual(r.status_code, 200)
        self.assertFalse(r.json()["dev_mode"])

    def test_correct_token_passes(self):
        r = self.client.post("/v1/score", json=payload(1), headers={"X-Sidecar-Token": TOKEN})
        self.assertEqual(r.status_code, 200)

    def test_reload_requires_token(self):
        self.assertEqual(self.client.post("/admin/reload").status_code, 403)
        r = self.client.post("/admin/reload", headers={"X-Sidecar-Token": TOKEN})
        self.assertEqual(r.status_code, 200)
        self.assertTrue(r.json()["reloaded"])


if __name__ == "__main__":
    unittest.main()
