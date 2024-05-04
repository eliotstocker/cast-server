FROM golang:1.22 as build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /cc-server

FROM gcr.io/distroless/static-debian12

COPY --from=build /cc-server /

CMD ["/cc-server"]