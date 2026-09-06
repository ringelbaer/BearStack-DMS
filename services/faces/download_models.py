"""Build/setup only. Download pinned OpenCV models and their license notices."""
import hashlib
import json
from pathlib import Path
import sys
from urllib.request import urlopen


def download(destination):
    lock = json.loads(Path(__file__).with_name("models.json").read_text())
    destination.mkdir(parents=True, exist_ok=True)
    for model in lock["models"]:
        url = f'https://media.githubusercontent.com/media/opencv/opencv_zoo/{lock["revision"]}/models/{model["path"]}'
        data = urlopen(url, timeout=120).read(64 * 1024 * 1024)
        if hashlib.sha256(data).hexdigest() != model["sha256"]:
            raise RuntimeError("Model checksum mismatch")
        (destination / Path(model["path"]).name).write_bytes(data)
        directory = model["path"].split("/")[0]
        license_url = f'https://raw.githubusercontent.com/opencv/opencv_zoo/{lock["revision"]}/models/{directory}/LICENSE'
        (destination / f"{directory}.LICENSE").write_bytes(urlopen(license_url, timeout=30).read())


if __name__ == "__main__":
    download(Path(sys.argv[1] if len(sys.argv) > 1 else "/models"))
