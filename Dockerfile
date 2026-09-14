FROM golang:1.26.3-bookworm AS build

WORKDIR /src

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -buildvcs=false -ldflags="-s -w" -o /server ./cmd/server

# CGO is disabled, so the binary is fully static. distroless/static is a few MB:
# no shell, CA certs included, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
COPY --from=build /src/application.yaml /application.yaml
EXPOSE 8080
ENTRYPOINT ["/server"]
CMD ["-config", "/application.yaml"]
