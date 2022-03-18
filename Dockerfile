FROM golang as builder

WORKDIR /go/src/app
COPY . .

RUN go build .

FROM gcr.io/distroless/base-debian10

WORKDIR /

COPY --from=builder /go/src/app/tcp-echo /tcp-echo
COPY --from=builder /go/src/app/fly.toml /fly.toml

CMD ["/tcp-echo"]