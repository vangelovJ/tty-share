FROM node:10.17.0-alpine AS node-builder
COPY . /tty-server
WORKDIR /tty-server
RUN cd frontend && \
    npm install && \
    node ./node_modules/webpack/bin/webpack.js

RUN mkdir -p /output/tty-server/frontend/public && \
    cp -r frontend/public /output/

FROM golang:1.13.1-alpine AS builder

RUN apk add git

COPY . /tty-server
COPY --from=node-builder /output /tty-server/frontend
WORKDIR /tty-server
RUN go get github.com/go-bindata/go-bindata/...

RUN go-bindata --prefix frontend/public/ -o tty-server/assets_bundle.go \
    frontend/public/404.css frontend/public/404.html \
    frontend/public/bootstrap.min.css frontend/public/index.html \
    frontend/public/invalid-session.html frontend/public/tty-receiver.in.html \
    frontend/public/tty-receiver.js
RUN mkdir out
RUN go build -o out/tty-server ./tty-server/pty_master.go \
    ./tty-server/server.go ./tty-server/server_main.go \
    ./tty-server/websockets_connection.go ./tty-server/assets_bundle.go

RUN mkdir -p /output && \
    mv out/tty-server /output/

FROM golang:1.13
COPY --from=builder /output /

EXPOSE "80"
ENTRYPOINT ["./tty-server"]
