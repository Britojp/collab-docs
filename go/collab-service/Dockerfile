FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /collab-service ./cmd/main.go

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /collab-service ./collab-service
EXPOSE 50052
CMD ["./collab-service"]
