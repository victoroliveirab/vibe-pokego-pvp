FROM golang:1.22-bookworm

WORKDIR /app

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
	build-essential \
	ffmpeg \
	tesseract-ocr \
	tesseract-ocr-eng \
	wget \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY go.mod ./
RUN go mod download

COPY . .

EXPOSE 8081

CMD ["go", "run", "./cmd/worker"]
