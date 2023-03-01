FROM golang:1.18-buster as builder
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o /app/az-wrapper

FROM mcr.microsoft.com/azure-cli
WORKDIR /
COPY --from=builder  /app/az-wrapper /
ENTRYPOINT ["/az-wrapper"]
