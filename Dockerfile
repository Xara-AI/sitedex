# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/sitedex ./cmd/sitedex

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/sitedex /usr/local/bin/sitedex

ENV SITEDEX_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["sitedex"]
CMD ["serve"]
