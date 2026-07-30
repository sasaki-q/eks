FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/main .

FROM --platform=linux/amd64 gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/main /main

CMD ["/main"]