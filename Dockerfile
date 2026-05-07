FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/tabhub ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache su-exec && adduser -D -H -u 10001 appuser
COPY --from=builder /out/tabhub /app/tabhub
COPY config.env /app/config.env
COPY favicon.ico /app/favicon.ico
COPY web /app/web
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN mkdir -p /app/data/uploads/wallpapers /app/data/uploads/icons && chown -R appuser:appuser /app
RUN chmod +x /app/docker-entrypoint.sh
EXPOSE 8080
ENV CONFIG_FILE=/app/config.env
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/tabhub"]
