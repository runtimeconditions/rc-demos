FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o /out/request-logger .

FROM gcr.io/distroless/static-debian12

COPY --from=build /out/request-logger /request-logger

ENTRYPOINT ["/request-logger"]
