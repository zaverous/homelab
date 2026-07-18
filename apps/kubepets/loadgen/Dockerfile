# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/kubepets-loadgen main.go

FROM scratch
COPY --from=build /out/kubepets-loadgen /kubepets-loadgen
ENTRYPOINT ["/kubepets-loadgen"]
