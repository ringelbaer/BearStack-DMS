"""Stateless, authenticated CPU face inference. No URLs, paths or database access."""
import os
# Set before importing native numeric runtimes.
os.environ["OPENBLAS_NUM_THREADS"] = "1"
os.environ["OMP_NUM_THREADS"] = "1"
os.environ["OPENCV_IO_MAX_IMAGE_PIXELS"] = str(1600 * 1600)
import hashlib
import hmac
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading

import cv2
import numpy as np

MODEL = "yunet-2023mar-sface-2021dec-v1"
MAX_BYTES = 8 * 1024 * 1024
MAX_FACES = 256


class Engine:
    def __init__(self, directory):
        lock = json.loads(Path(__file__).with_name("models.json").read_text())
        for model in lock["models"]:
            path = directory / Path(model["path"]).name
            if hashlib.sha256(path.read_bytes()).hexdigest() != model["sha256"]:
                raise RuntimeError("Model checksum mismatch")
        cv2.setNumThreads(1)
        self.detector = cv2.FaceDetectorYN.create(str(directory / "face_detection_yunet_2023mar.onnx"), "", (320, 320), 0.9, 0.3, 5000)
        self.recognizer = cv2.FaceRecognizerSF.create(str(directory / "face_recognition_sface_2021dec.onnx"), "")

    def analyze(self, body):
        if not body or len(body) > MAX_BYTES:
            raise ValueError("Invalid image size")
        image = cv2.imdecode(np.frombuffer(body, dtype=np.uint8), cv2.IMREAD_COLOR | cv2.IMREAD_IGNORE_ORIENTATION)
        if image is None:
            raise ValueError("Invalid JPEG")
        height, width = image.shape[:2]
        if max(height, width) > 1600:
            raise ValueError("Image exceeds 1600 pixels")
        self.detector.setInputSize((width, height))
        _, detections = self.detector.detect(image)
        result = []
        if detections is not None:
            if len(detections) > MAX_FACES:
                raise ValueError("Too many faces")
            for face in detections:
                x, y, w, h = map(float, face[:4])
                left, top = max(0.0, x), max(0.0, y)
                right, bottom = min(float(width), x + w), min(float(height), y + h)
                if right - left < 12 or bottom - top < 12:
                    continue
                aligned = self.recognizer.alignCrop(image, face)
                feature = self.recognizer.feature(aligned).flatten()
                norm = float(np.linalg.norm(feature))
                if feature.size != 128 or not np.isfinite(feature).all() or norm <= 0:
                    raise ValueError("Invalid feature")
                result.append({"x": left / width, "y": top / height, "width": (right-left) / width, "height": (bottom-top) / height, "confidence": float(face[-1]), "embedding": (feature/norm).tolist()})
        return {"model": MODEL, "faces": result}


class Server(ThreadingHTTPServer):
    daemon_threads = True
    request_queue_size = 8

    def __init__(self, address, engine, token):
        super().__init__(address, Handler)
        self.engine = engine
        self.token = token
        self.inference = threading.BoundedSemaphore(1)
        self.connections = threading.BoundedSemaphore(8)

    def process_request(self, request, client_address):
        if not self.connections.acquire(blocking=False):
            self.shutdown_request(request)
            return
        try:
            super().process_request(request, client_address)
        except BaseException:
            self.connections.release()
            raise

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self.connections.release()


class Handler(BaseHTTPRequestHandler):
    server_version = "BearStackFaces/1"

    def setup(self):
        super().setup()
        self.connection.settimeout(30)

    def log_message(self, *_):
        pass  # Never log tokens, image payloads or embeddings.

    def reply(self, status, body):
        data = json.dumps(body, allow_nan=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(data)

    def authenticated(self):
        expected = ("Bearer " + self.server.token).encode()
        actual = self.headers.get("Authorization", "").encode()
        if not hmac.compare_digest(expected, actual):
            self.reply(401, {"error": "Unauthorized"})
            return False
        return True

    def do_GET(self):
        if not self.authenticated():
            return
        if self.path != "/health":
            self.reply(404, {"error": "Not found"})
            return
        self.reply(200, {"ready": True, "protocol": 1, "model": MODEL})

    def do_POST(self):
        if not self.authenticated():
            return
        if self.path != "/v1/analyze":
            self.reply(404, {"error": "Not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > MAX_BYTES or self.headers.get("Transfer-Encoding"):
            self.reply(413, {"error": "Invalid body size"})
            return
        if self.headers.get("Content-Type") != "image/jpeg":
            self.reply(415, {"error": "JPEG required"})
            return
        if not self.server.inference.acquire(blocking=False):
            self.reply(429, {"error": "Busy"})
            return
        try:
            body = self.rfile.read(length)
            if len(body) != length:
                raise ValueError("Incomplete body")
            result = self.server.engine.analyze(body)
            self.reply(200, result)
        except (ValueError, cv2.error):
            self.reply(422, {"error": "Image cannot be analyzed"})
        finally:
            self.server.inference.release()


if __name__ == "__main__":
    token = os.environ.get("BEARSTACK_FACE_SERVICE_TOKEN", "")
    if len(token) < 32 or "\n" in token or "\r" in token:
        raise SystemExit("BEARSTACK_FACE_SERVICE_TOKEN must contain at least 32 characters")
    engine = Engine(Path(os.environ.get("BEARSTACK_FACE_MODELS_DIR", "/models")))
    Server((os.environ.get("BEARSTACK_FACE_BIND", "127.0.0.1"), 8091), engine, token).serve_forever()
