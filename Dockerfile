FROM golang:1.25 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/spl-playground ./cmd/playground

FROM gcr.io/distroless/static-debian12:nonroot

ENV PLAYGROUND_ADDR=:8080
EXPOSE 8080

COPY --from=build /out/spl-playground /usr/local/bin/spl-playground
ENTRYPOINT ["/usr/local/bin/spl-playground"]
