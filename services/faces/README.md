# BearStack face inference service

Optional, CPU-only, stateless Python 3.12+ service. BearStack owns the job queue,
person groups and embeddings in its photo database. This process receives only
upright JPEG bytes and returns bounded face boxes and normalized 128-dimensional
vectors. No filesystem paths or external image URLs are accepted. Python release artifacts
are pinned with pip-enforced SHA-256 hashes in `requirements.txt`.

## Run without Docker

```sh
python3 -m venv .venv-faces
.venv-faces/bin/pip install -r services/faces/requirements.txt
.venv-faces/bin/python services/faces/download_models.py /your/model/directory
export BEARSTACK_FACE_MODELS_DIR=/your/model/directory
export BEARSTACK_FACE_SERVICE_TOKEN='your-random-token-of-at-least-32-characters'
.venv-faces/bin/python services/faces/server.py
```

The native default is `127.0.0.1:8091`. Configure BearStack with
`BEARSTACK_PHOTOS_FACE_SERVICE_URL=http://127.0.0.1:8091` and the same token in
`BEARSTACK_PHOTOS_FACE_SERVICE_TOKEN`, then enable recognition in the UI.
Use HTTPS for a service on another host; private image traffic must stay within
infrastructure you control. BearStack does not use environment HTTP proxies or
follow redirects for inference requests.

Compose uses `docker compose --profile faces up -d --build`; set the shared token
in `.env` first. The service runs unprivileged with a read-only filesystem, no photo
or database mounts, one inference thread, 0.5 CPU and 1 GiB RAM. Disabling recognition
in BearStack stops analysis, not the independently managed service container.
The service fails startup for missing tokens, missing models or checksum mismatches.

## Models and licenses

`models.json` pins the OpenCV Zoo commit and the SHA-256 hashes from its LFS objects.
`download_models.py` is a setup/build command only. It also includes the YuNet MIT
and SFace Apache-2.0 notices alongside the model files. Runtime verifies both hashes.
Model upgrades require a new protocol model ID and matching BearStack client version;
vectors from different generations are never compared.

- https://github.com/opencv/opencv_zoo/tree/47534e27c9851bb1128ccc0102f1145e27f23f98/models/face_detection_yunet
- https://github.com/opencv/opencv_zoo/tree/47534e27c9851bb1128ccc0102f1145e27f23f98/models/face_recognition_sface
- https://docs.opencv.org/4.13.0/d0/dd4/tutorial_dnn_face.html

Detection uses confidence >= 0.9. Automatic grouping requires cosine similarity
>= 0.55 and a margin >= 0.08 over the second distinct candidate person, after an
HNSW candidate search and exact re-scoring. These are conservative engineering
defaults, not a claim of calibrated accuracy for every population or photo collection.
At most five references per person are selected, prioritizing manual assignments
and detector confidence. Low-confidence identity matches remain unnamed groups.

Limits: 8 MiB compressed request, 1,600-pixel maximum edge, 256 faces, one concurrent
inference and eight concurrent HTTP connections. Excess requests receive 429;
invalid images receive 422. Input decoding is additionally pixel-limited in OpenCV.
The browser receives none of the internal vectors. See `openapi.yaml` for the
internal service protocol; the root OpenAPI file describes BearStack's user routes.

## Tests

```sh
BEARSTACK_TEST_FACE_MODELS_DIR=/your/model/directory \
  .venv-faces/bin/python -m unittest discover -s services/faces/tests -v
go test ./internal/facerec ./internal/photos ./internal/server
go test -race ./internal/facerec ./internal/photos ./internal/server
BEARSTACK_FACE_SCALE_TEST=1 go test ./internal/photos -run TestFaceScaleMillion -count=1 -v
PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright
```

Protocol tests use a local HTTP server. Real-model tests use the documented NASA
fixture; without `BEARSTACK_TEST_FACE_MODELS_DIR` they are explicitly skipped. This
small fixture set verifies integration and same-image consistency, not population
accuracy. The opt-in scale test creates 100,000 photo records and one million face
records (about 1 GiB temporary disk), then measures the 50,000-reference index and
person-gallery queries during synthetic background queue writes.
