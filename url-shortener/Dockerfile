FROM golang:1.27 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# Before the source copy: goose only changes when go.mod does.
RUN CGO_ENABLED=0 go build -o /bin/goose github.com/pressly/goose/v3/cmd/goose

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /bin/url_shortener ./cmd

FROM scratch
WORKDIR /src
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/url_shortener /bin/url_shortener
COPY --from=builder /bin/goose /bin/goose
COPY config ./config
COPY internal/database/migrations /migrations

# Numeric UID/GID: scratch has no /etc/passwd to resolve a name against.
# 65534:65534 is the conventional "nobody". Files stay root-owned and are
# only read, so the process cannot modify its own binary or config.
USER 65534:65534

CMD ["/bin/url_shortener", "--environment", "local"]
