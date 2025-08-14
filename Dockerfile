FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/awesomegen ./cmd/awesomegen

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/awesomegen /usr/local/bin/awesomegen
ENTRYPOINT ["/usr/local/bin/awesomegen"]
