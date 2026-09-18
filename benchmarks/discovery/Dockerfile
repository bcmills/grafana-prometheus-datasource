FROM golang:1.26.7-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dataset ./cmd/dataset

FROM alpine:3.21
RUN apk add --no-cache wget
COPY --from=builder /out/dataset /usr/local/bin/dataset
EXPOSE 9115
ENTRYPOINT ["/usr/local/bin/dataset"]
