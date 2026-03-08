FROM golang:1.22-bookworm

WORKDIR /app

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
	build-essential \
	ffmpeg \
	wget \
	ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY go.mod ./
RUN go mod download

COPY . .

EXPOSE 8080

CMD ["go", "run", "./cmd/server"]
