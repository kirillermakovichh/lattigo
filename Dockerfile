FROM golang:1.26-alpine3.23 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/blnode ./cmd/blnode && \
    mkdir -p /out/data && chown 65532:65532 /out/data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/blnode /usr/local/bin/blnode
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
WORKDIR /data
EXPOSE 7070 7071
ENTRYPOINT ["/usr/local/bin/blnode"]
CMD ["--config", "/config/node.json"]
