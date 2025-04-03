ARG VERSION=unknown
ARG CREATED="an unknown date"
ARG COMMIT=unknown

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o gangplank -ldflags="-w -s" .

FROM alpine:3

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/gangplank /app/gangplank

RUN adduser -D -H -h /app gangplank

USER gangplank

ENTRYPOINT ["/app/gangplank"]

CMD ["daemon", "--poll"]