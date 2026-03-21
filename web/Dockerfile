FROM golang:1.24-bookworm

WORKDIR /app/web

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
	build-essential \
	ffmpeg \
	wget \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY shared /app/shared
COPY web/go.mod web/go.sum ./
RUN go mod download

COPY web .

EXPOSE 8080

CMD ["go", "run", "./cmd/server"]
