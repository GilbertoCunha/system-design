FROM golang:1.27 AS builder
WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -o /bin/url_shortener ./cmd/server.go

FROM scratch
COPY --from=builder /bin/url_shortener /bin/url_shortener
CMD ["/bin/url_shortener"]
