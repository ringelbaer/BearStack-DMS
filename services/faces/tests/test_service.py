import http.client
import json
import os
from pathlib import Path
import sys
import threading
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from server import Engine, MODEL, Server, MAX_BYTES, MAX_FACES, recognition_quality
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


class QualityTests(unittest.TestCase):
    def test_small_and_blurry_faces_do_not_seed_references(self):
        yy, xx = np.indices((112, 112))
        textured = np.repeat((((xx // 3 + yy // 3) % 2) * 255).astype(np.uint8)[..., None], 3, axis=2)
        self.assertFalse(recognition_quality(textured, 47)["reference_eligible"])
        self.assertTrue(recognition_quality(textured, 48)["reference_eligible"])
        flat = np.full((112, 112, 3), 128, dtype=np.uint8)
        quality = recognition_quality(flat, 100)
        self.assertEqual(quality["sharpness"], 0)
        self.assertFalse(quality["reference_eligible"])

    def test_warp_padding_does_not_inflate_sharpness(self):
        image = np.zeros((112, 112, 3), dtype=np.uint8)
        image[10:-10, 10:-10] = 128
        self.assertFalse(recognition_quality(image, 100)["reference_eligible"])


class DetectionScaleTests(unittest.TestCase):
    def engine(self, native, overview):
        class Detector:
            def __init__(self):
                self.sizes = []
            def setInputSize(self, size):
                self.size = size
            def detect(self, image):
                self.sizes.append(self.size)
                return None, native if len(self.sizes) == 1 else overview
        engine = Engine.__new__(Engine)
        engine.detector = Detector()
        engine.overview_detector = engine.detector
        return engine

    def test_overview_maps_all_landmarks_and_keeps_native_detections(self):
        native = np.array([[100, 100, 30, 40, 110, 110, 120, 110, 115, 120, 110, 130, 120, 130, .95]], dtype=np.float32)
        duplicate = native.copy()
        duplicate[:, 0:14:2] *= 320 / 1600
        duplicate[:, 1:14:2] *= 213 / 1066
        large = np.array([[150, 50, 90, 110, 175, 80, 205, 80, 190, 100, 180, 130, 200, 130, .93]], dtype=np.float32)
        engine = self.engine(native, np.concatenate([duplicate, large]))
        faces = engine.detect_faces(np.zeros((1066, 1600, 3), dtype=np.uint8))
        self.assertEqual(engine.detector.sizes, [(1600, 1066), (320, 213)])
        self.assertEqual(len(faces), 2)
        np.testing.assert_array_equal(faces[0], native[0])
        expected = large[0].copy()
        expected[0:14:2] *= 5
        expected[1:14:2] *= 1066 / 213
        np.testing.assert_allclose(faces[1], expected)
        np.testing.assert_array_equal(large[0, :4], [150, 50, 90, 110])

    def test_no_redundant_pass_for_small_inputs(self):
        engine = self.engine(None, None)
        self.assertEqual(engine.detect_faces(np.zeros((213, 320, 3), dtype=np.uint8)), [])
        self.assertEqual(engine.detector.sizes, [(320, 213)])

    def test_narrow_overview_is_padded_and_limits_are_preserved(self):
        engine = self.engine(None, None)
        self.assertEqual(engine.detect_faces(np.zeros((80, 1600, 3), dtype=np.uint8)), [])
        self.assertEqual(engine.detector.sizes, [(1600, 80), (320, 32)])
        for native, overview in [(np.zeros((MAX_FACES+1, 15)), None), (None, np.zeros((MAX_FACES+1, 15)))]:
            with self.assertRaisesRegex(ValueError, "Too many faces"):
                self.engine(native, overview).detect_faces(np.zeros((512, 512, 3), dtype=np.uint8))


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
            self.assertGreater(face["quality"]["face_pixels"], 0)
            self.assertTrue(np.isfinite(face["quality"]["sharpness"]))

    def test_original_crop_recovers_small_face_identity_detail(self):
        # The very same source face shrinks to about 28 px in the bounded
        # preview. Re-detecting its original crop recovers about 92 px without
        # changing weights or increasing the whole-image detection resolution.
        source = np.zeros((2400, 4800, 3), dtype=np.uint8)
        source[200:712, 700:1212] = self.image
        preview = cv2.resize(source, (1600, 800), interpolation=cv2.INTER_AREA)
        detected = self.analyze(preview)
        self.assertEqual(len(detected), 1)
        face = detected[0]
        self.assertFalse(face["quality"]["reference_eligible"])
        width, height = source.shape[1], source.shape[0]
        cx = (face["x"] + face["width"] / 2) * width
        cy = (face["y"] + face["height"] / 2) * height
        half = .9 * max(face["width"] * width, face["height"] * height)
        left, top = max(0, int(np.floor(cx-half))), max(0, int(np.floor(cy-half)))
        right, bottom = min(width, int(np.ceil(cx+half))), min(height, int(np.ceil(cy+half)))
        refined = self.analyze(source[top:bottom, left:right])
        self.assertEqual(len(refined), 1)
        self.assertTrue(refined[0]["quality"]["reference_eligible"])
        baseline = np.array(self.analyze(self.image)[0]["embedding"])
        preview_score = float(baseline @ np.array(face["embedding"]))
        refined_score = float(baseline @ np.array(refined[0]["embedding"]))
        self.assertGreater(refined_score, preview_score + .1)
        self.assertGreater(refined_score, .9)

    def test_small_rotation_consistency(self):
        transform = cv2.getRotationMatrix2D((256, 256), 10, 1)
        tilted = cv2.warpAffine(self.image, transform, (512, 512))
        baseline, rotated = self.analyze(self.image), self.analyze(tilted)
        self.assertEqual(len(rotated), 1)
        self.assertGreater(float(np.array(baseline[0]["embedding"]) @ np.array(rotated[0]["embedding"])), 0.55)

    def test_large_portrait_and_mixed_face_sizes(self):
        # Derived in memory from the public fixture; no private photos in tests.
        portrait = cv2.resize(self.image[30:210, 140:320], (1024, 1024), interpolation=cv2.INTER_CUBIC)
        self.engine.detector.setInputSize((1024, 1024))
        _, native = self.engine.detector.detect(portrait)
        self.assertTrue(native is None or len(native) == 0)
        faces = self.analyze(portrait)
        self.assertEqual(len(faces), 1)
        self.assertGreater(faces[0]["quality"]["face_pixels"], 300)
        self.assertTrue(faces[0]["quality"]["reference_eligible"])
        baseline = np.array(self.analyze(self.image)[0]["embedding"])
        self.assertGreater(float(baseline @ np.array(faces[0]["embedding"])), .55)
        mixed = np.zeros((1024, 1536, 3), dtype=np.uint8)
        mixed[:, :1024] = portrait
        mixed[256:768, 1024:] = self.image
        faces = self.analyze(mixed)
        self.assertEqual(len(faces), 2)
        pixels = sorted(face["quality"]["face_pixels"] for face in faces)
        self.assertLess(pixels[0], 150)
        self.assertGreater(pixels[1], 300)

    def test_checksum_failure(self):
        import tempfile
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "face_detection_yunet_2023mar.onnx").write_bytes(b"bad")
            with self.assertRaisesRegex(RuntimeError, "checksum"):
                Engine(Path(directory))
