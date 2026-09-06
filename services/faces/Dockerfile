FROM python:3.12-slim-bookworm AS models
WORKDIR /build
COPY services/faces/download_models.py services/faces/models.json ./
RUN python download_models.py /models

FROM python:3.12-slim-bookworm
WORKDIR /app
COPY services/faces/requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY --from=models /models /models
COPY services/faces/server.py services/faces/models.json ./
USER 65534:65534
ENV BEARSTACK_FACE_BIND=0.0.0.0 PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
EXPOSE 8091
HEALTHCHECK --interval=60s --timeout=10s --start-period=30s CMD python -c "import os,urllib.request; r=urllib.request.Request('http://127.0.0.1:8091/health',headers={'Authorization':'Bearer '+os.environ['BEARSTACK_FACE_SERVICE_TOKEN']}); urllib.request.urlopen(r,timeout=5)"
CMD ["python", "server.py"]
