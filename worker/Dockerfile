FROM golang:1.22-bookworm

WORKDIR /app/worker

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
	build-essential \
	ffmpeg \
	tesseract-ocr \
	tesseract-ocr-eng \
	wget \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY shared /app/shared
COPY worker/go.mod worker/go.sum ./
RUN go mod download

COPY worker .

EXPOSE 8081

CMD ["go", "run", "./cmd/worker"]
