import http.client
import json
import os
from pathlib import Path
import sys
import threading
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from server import Engine, MODEL, Server, MAX_BYTES
import cv2
import numpy as np


class ProtocolTests(unittest.TestCase):
    def setUp(self):
        class FakeEngine:
            def analyze(self, body):
                if body != b"jpeg":
                    raise ValueError("invalid")
                return {"model": MODEL, "faces": []}
        self.server = Server(("127.0.0.1", 0), FakeEngine(), "t" * 32)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()

    def request(self, method, path, body=None, auth=True, **headers):
        client = http.client.HTTPConnection(*self.server.server_address, timeout=5)
        if auth:
            headers["Authorization"] = "Bearer " + "t" * 32
        headers.setdefault("Content-Type", "image/jpeg")
        client.request(method, path, body, headers)
        response = client.getresponse()
        status, data = response.status, json.loads(response.read())
        client.close()
        return status, data

    def test_auth_health_and_empty_result(self):
        self.assertEqual(self.request("GET", "/health", auth=False)[0], 401)
        status, data = self.request("GET", "/health")
        self.assertEqual((status, data["model"], data["protocol"]), (200, MODEL, 1))
        self.assertEqual(self.request("POST", "/v1/analyze", b"jpeg"), (200, {"model": MODEL, "faces": []}))

    def test_invalid_and_busy(self):
        self.assertEqual(self.request("POST", "/v1/analyze", b"broken")[0], 422)
        self.assertEqual(self.request("POST", "/v1/analyze", b"x", **{"Content-Type": "text/plain"})[0], 415)
        self.assertEqual(self.request("POST", "/v1/analyze", b"", **{"Content-Length": str(MAX_BYTES+1)})[0], 413)
        self.server.inference.acquire()
        try:
            self.assertEqual(self.request("POST", "/v1/analyze", b"jpeg")[0], 429)
        finally:
            self.server.inference.release()


@unittest.skipUnless(os.environ.get("BEARSTACK_TEST_FACE_MODELS_DIR"), "Set BEARSTACK_TEST_FACE_MODELS_DIR for real-model tests")
class ModelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.engine = Engine(Path(os.environ["BEARSTACK_TEST_FACE_MODELS_DIR"]))
        cls.image = cv2.imread(str(Path(__file__).parent / "fixtures/astronaut.png"))
        assert cls.image is not None

    def analyze(self, image):
        ok, encoded = cv2.imencode(".jpg", image)
        self.assertTrue(ok)
        return self.engine.analyze(encoded.tobytes())["faces"]

    def test_blank_and_broken(self):
        self.assertEqual(self.analyze(np.zeros((512, 512, 3), dtype=np.uint8)), [])
        with self.assertRaises(ValueError):
            self.engine.analyze(b"not an image")

    def test_multiple_faces_and_resize_consistency(self):
        single = self.analyze(self.image)
        self.assertEqual(len(single), 1)
        pair = self.analyze(np.concatenate([self.image, self.image], axis=1))
        self.assertEqual(len(pair), 2)
        smaller = self.analyze(cv2.resize(self.image, (384, 384)))
        self.assertEqual(len(smaller), 1)
        a, b = np.array(single[0]["embedding"]), np.array(smaller[0]["embedding"])
        self.assertGreater(float(a @ b), 0.55)
        for face in single + pair + smaller:
            self.assertEqual(len(face["embedding"]), 128)
            self.assertAlmostEqual(float(np.linalg.norm(face["embedding"])), 1, places=5)
            self.assertGreaterEqual(face["x"], 0)
            self.assertLessEqual(face["x"] + face["width"], 1)

    def test_small_rotation_consistency(self):
        transform = cv2.getRotationMatrix2D((256, 256), 10, 1)
        tilted = cv2.warpAffine(self.image, transform, (512, 512))
        baseline, rotated = self.analyze(self.image), self.analyze(tilted)
        self.assertEqual(len(rotated), 1)
        self.assertGreater(float(np.array(baseline[0]["embedding"]) @ np.array(rotated[0]["embedding"])), 0.55)

    def test_checksum_failure(self):
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "face_detection_yunet_2023mar.onnx").write_bytes(b"bad")
            with self.assertRaisesRegex(RuntimeError, "checksum"):
                Engine(Path(directory))
