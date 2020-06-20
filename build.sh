#!/bin/bash
set -x
cp -rp /home/venven/Go/tty-share /tty-server
cd /tty-server
cd frontend
npm install 
node ./node_modules/webpack/bin/webpack.js
mkdir -p /output/tty-server/frontend/public
cp -r public /output/

mkdir -p /tty-server-1
cp -rp /home/venven/Go/tty-share/* /tty-server-1
cp -rp /output/* /tty-server-1/frontend/
cd /tty-server-1
go-bindata --prefix frontend/public/ -o tty-server/assets_bundle.go \
    frontend/public/404.css frontend/public/404.html \
    frontend/public/bootstrap.min.css frontend/public/index.html \
    frontend/public/invalid-session.html frontend/public/tty-receiver.in.html \
    frontend/public/tty-receiver.js
mkdir out
go build -o out/tty-server ./tty-server/pty_master.go \
    ./tty-server/server.go ./tty-server/server_main.go \
    ./tty-server/websockets_connection.go ./tty-server/assets_bundle.go

mv out/tty-server /tmp/tty-server
