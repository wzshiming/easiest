FROM golang:alpine AS builder
WORKDIR /go/src/github.com/wzshiming/easiest/
COPY . .
ENV CGO_ENABLED=0
RUN go install ./cmd/easiest

FROM alpine
EXPOSE 80
EXPOSE 443
COPY --from=builder /go/bin/easiest /usr/local/bin/
ENTRYPOINT [ "/usr/local/bin/easiest" ]
