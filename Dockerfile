FROM golang:1.19 as build

ENV APP_HOME /go/src/tt-golang
RUN mkdir -p "$APP_HOME"
WORKDIR "$APP_HOME"
COPY . .
RUN go mod download
RUN go mod verify
RUN go build -o webservice ./cmd/webservice.go

FROM golang:1.19
ENV APP_HOME /go/src/tt-golang
RUN mkdir -p "$APP_HOME"
WORKDIR "$APP_HOME"
COPY --from=build "$APP_HOME"/webservice $APP_HOME

EXPOSE 3000
CMD ["./webservice"]
