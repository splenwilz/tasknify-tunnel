FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /devtunnel-server ./cmd/devtunnel-server

FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /devtunnel-server /usr/local/bin/devtunnel-server

EXPOSE 8001

ENTRYPOINT ["/usr/local/bin/devtunnel-server"]
