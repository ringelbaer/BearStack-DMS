# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26-trixie AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
	go build -trimpath -ldflags="-s -w" -o /out/bearstack ./cmd/bearstack

FROM debian:trixie-slim AS runtime

RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		ca-certificates \
		ffmpeg \
		libreoffice-writer \
		poppler-utils \
		tesseract-ocr \
		tesseract-ocr-deu \
		tesseract-ocr-eng \
	&& rm -rf /var/lib/apt/lists/*

RUN groupadd --system --gid 10001 bearstack \
	&& useradd --system --uid 10001 --gid bearstack \
		--home-dir /var/lib/bearstack \
		--no-create-home \
		--shell /usr/sbin/nologin \
		bearstack \
	&& mkdir -p /var/lib/bearstack \
	&& chown bearstack:bearstack /var/lib/bearstack

COPY --from=build /out/bearstack /usr/local/bin/bearstack

ENV BEARSTACK_ADDR=0.0.0.0:8080 \
	BEARSTACK_DATA_DIR=/var/lib/bearstack

VOLUME ["/var/lib/bearstack"]
EXPOSE 8080

USER bearstack:bearstack
ENTRYPOINT ["/usr/local/bin/bearstack"]
