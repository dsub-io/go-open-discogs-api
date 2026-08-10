FROM golang:1.26.5-alpine3.23 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=development
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X github.com/dsub-io/go-open-discogs-api/internal/buildinfo.Version=${VERSION}" \
    -o /out/go-open-discogs-api \
    .

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/go-open-discogs-api /go-open-discogs-api

USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/go-open-discogs-api"]
