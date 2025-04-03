ARG VERSION=unknown
ARG CREATED="an unknown date"
ARG COMMIT=unknown

FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w \
	-X 'cmd.version=$VERSION' \
	-X 'cmd.created=$CREATED' \
	-X 'cmd.commit=$COMMIT' \
    " -o gangplank

FROM alpine:3

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/gangplank /app/gangplank

ENTRYPOINT ["/app/gangplank"]

CMD ["daemon", "--poll"]