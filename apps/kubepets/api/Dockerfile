# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/kubepets-api .

FROM scratch
COPY --from=build /out/kubepets-api /kubepets-api
EXPOSE 8080
ENTRYPOINT ["/kubepets-api"]
