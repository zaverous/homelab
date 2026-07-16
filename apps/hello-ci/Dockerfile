# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/hello-ci main.go

FROM scratch
COPY --from=build /out/hello-ci /hello-ci
EXPOSE 8080
ENTRYPOINT ["/hello-ci"]
