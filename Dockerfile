# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN make build
RUN go install github.com/steveyegge/beads/cmd/bd@latest

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && addgroup -S gastown \
    && adduser -S -G gastown -u 10001 gastown

ENV GT_ROOT=/gt
WORKDIR /gt

COPY --from=builder /src/gt /usr/local/bin/gt
COPY --from=builder /go/bin/bd /usr/local/bin/bd

USER gastown
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/usr/local/bin/gt", "version"]

ENTRYPOINT ["/usr/local/bin/gt"]
CMD ["gui", "--port", "8080"]
