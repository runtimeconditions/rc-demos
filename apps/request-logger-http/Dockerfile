FROM golang:1.25.3-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY *.go ./

RUN go mod download
RUN go build -o /out/request-logger .

FROM gcr.io/distroless/static-debian12

COPY --from=build /out/request-logger /request-logger

ENTRYPOINT ["/request-logger"]
