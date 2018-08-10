FROM golang:1.9 AS build

ARG VERSION=latest

WORKDIR /go/src/res-pack
COPY . .
RUN go install -ldflags "-s -w -X main.version=$VERSION" res-pack

FROM gcr.io/google_containers/ubuntu-slim:0.14
COPY --from=build /go/bin/res-pack /usr/bin/res-pack

ENTRYPOINT ["res-pack"]
