# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
# ca-certificates: the scratch image below has no root CA bundle, so without this
# Go can't verify ANY outbound TLS (e.g. smtp.resend.com for verification mail) -
# every dial fails with "x509: certificate signed by unknown authority".
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/kubepets-api .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/kubepets-api /kubepets-api
EXPOSE 8080
ENTRYPOINT ["/kubepets-api"]
