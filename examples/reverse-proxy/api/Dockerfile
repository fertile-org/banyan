FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -o api .

FROM alpine:3.21
COPY --from=builder /app/api /usr/local/bin/api
EXPOSE 8080
CMD ["api"]
