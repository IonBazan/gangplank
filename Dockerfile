# The builder runs on the build machine and cross-compiles, which is much faster than emulating each platform.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=unknown
ARG CREATED=unknown
ARG COMMIT=unknown

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build -trimpath -ldflags="-s -w \
	-X 'github.com/IonBazan/gangplank/cmd.version=${VERSION}' \
	-X 'github.com/IonBazan/gangplank/cmd.created=${CREATED}' \
	-X 'github.com/IonBazan/gangplank/cmd.commit=${COMMIT}' \
	" -o gangplank

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/gangplank /app/gangplank

WORKDIR /app

ENTRYPOINT ["/app/gangplank"]

CMD ["daemon", "--poll"]
