# BearStack face inference service

Optional, CPU-only, stateless Python 3.12+ service. BearStack owns the job queue,
person groups and embeddings in its photo database. This process receives only
upright JPEG bytes and returns bounded face boxes and normalized 128-dimensional
vectors. No filesystem paths or external image URLs are accepted. Python release artifacts
are pinned with pip-enforced SHA-256 hashes in `requirements.txt`.

Detection runs at the supplied resolution and, for inputs larger than 320 pixels,
also on a 320-pixel overview. YuNet's documented training scale is approximately
10–300 face pixels, so the overview recovers large portraits while the first pass
retains small faces. It runs even when other faces were already found. Overlapping
candidates (IoU > 0.3) are suppressed in favor of the original pass. All five
landmarks and boxes are mapped back using the actual width/height scale factors;
SFace alignment, embeddings and quality use the original supplied image. The
extra pass processes at most 320 × 320 pixels. Confidence stays at 0.9; model ID,
weights and embedding compatibility are unchanged. Rebuild/restart this service
and request analysis again for photos previously processed without detections.
With Compose: `docker compose --profile faces up -d --build faces`.

Responses include additive `quality` metadata: the shorter detected face side in
input pixels, central aligned-crop Laplacian sharpness, and `reference_eligible`.
The current engineering floors are 48 face pixels and sharpness 20. Weak faces
remain visible for manual assignment but do not automatically seed references;
explicit favorites can still be retained as references. Detector confidence is
separate from recognition quality. Legacy service responses without `quality`
remain compatible. These floors are not a population accuracy calibration.

BearStack can refine up to eight faces smaller than 80 preview pixels using
original-resolution crops when at least 1.5 times as much detail can be recovered.
The original is decoded once for all selected crops, with the existing 40 MP
limit and decode concurrency bound. Only JPEG crops are sent through the same
`/v1/analyze` endpoint; each retains the 1600-pixel and 8 MiB limits and the total
crop payload per photo is bounded to 16 MiB. Upright coordinates are mapped back
through EXIF orientation, and original detection boxes stay unchanged. An
ambiguous crop, insufficient quality or unavailable refinement keeps the initial
result. Source replacement, privacy changes and cancellation abort processing.
Refinement uses the same SFace alignment, weights and embedding space, so the
protocol model ID is unchanged and existing vectors remain comparable.

## Run without Docker

```sh
python3 -m venv .venv-faces
.venv-faces/bin/pip install -r services/faces/requirements.txt
.venv-faces/bin/python services/faces/download_models.py /your/model/directory
export BEARSTACK_FACE_MODELS_DIR=/your/model/directory
export BEARSTACK_FACE_SERVICE_TOKEN='your-random-token-of-at-least-32-characters'
.venv-faces/bin/python services/faces/server.py
```

If model downloads fail on macOS with `CERTIFICATE_VERIFY_FAILED`, the Python
installation may be missing its default CA bundle. When `/etc/ssl/cert.pem`
exists, use the system CA bundle for the download:

```sh
SSL_CERT_FILE=/etc/ssl/cert.pem .venv-faces/bin/python services/faces/download_models.py /your/model/directory
```

TLS certificate verification and model checksum validation remain enabled.

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
>= 0.55 and a margin >= 0.08 over the second distinct candidate person, with exact
comparison of the selected references for every person. Retrospective assignment
to a confirmed named person requires >= 0.62 and a margin >= 0.10; possible group
merges from >= 0.45 are offered for review. These are conservative engineering
defaults, not a claim of calibrated accuracy for every population or photo collection.
The reference limit is configurable in BearStack (1–100, default 30), prioritizing
manual assignments and detector confidence. It does not yet balance capture years.
Changes rebuild references from stored vectors before the next analysis, with
restartable checkpoints and no image re-analysis. Duplicate references cannot
crowd a competing person out of comparison. Low-confidence identity matches
remain unnamed groups.

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
BEARSTACK_TEST_FACE_MODELS_DIR=/your/model/directory \
  BEARSTACK_TEST_FACE_PYTHON="$PWD/.venv-faces/bin/python" \
  go test ./internal/photos -run TestFaceRefinementRealModel -count=1 -v
BEARSTACK_FACE_SCALE_TEST=1 go test ./internal/photos -run TestFaceScaleMillion -count=1 -v
PLAYWRIGHT_BROWSER_CHANNEL=chromium make test-playwright
```

Protocol tests use a local HTTP server. Real-model tests use the documented NASA
fixture; without `BEARSTACK_TEST_FACE_MODELS_DIR` they are explicitly skipped. This
small fixture set verifies integration, same-image consistency and improved
original-crop detail for a small detected face, not population accuracy. Quality
unit tests also verify that tiny or smooth crops cannot seed automatic references.
The opt-in scale test creates 100,000 photo records and one million face
records (about 1 GiB temporary disk), then measures the 300,000-reference index and
person-gallery queries during synthetic background queue writes.
