FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
# Initialize a simple module
RUN go mod init gateway && go build -o gateway main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/gateway .

EXPOSE 8080
CMD ["./gateway"]
