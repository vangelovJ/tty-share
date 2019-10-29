DEPS=github.com/creack/pty github.com/sirupsen/logrus golang.org/x/crypto/ssh/terminal github.com/gorilla/mux github.com/gorilla/websocket github.com/go-bindata/go-bindata/...
DEST_DIR=./out
TTY_SERVER=$(DEST_DIR)/tty-server

# We need to make sure the assets_bundle is in the list only onces in both these two special cases:
# a) first time, when the assets_bundle.go is generated, and b) when it's already existing there,
# but it has to be re-generated.
# Unfortunately, the assets_bundle.go seems to have to be in the same folder as the rest of the
# server sources, so that's why all this mess
TTY_SERVER_SRC=$(filter-out ./tty-server/assets_bundle.go, $(wildcard ./tty-server/*.go)) ./tty-server/assets_bundle.go
COMMON_SRC=$(wildcard ./common/*go)
TTY_SERVER_ASSETS=$(wildcard frontend/public/*)

## Build both the server and the tty-share
all: get-deps $(TTY_SERVER)
	@echo "All done"

get-deps:
	go get $(DEPS)

# Building the server and tty-share
$(TTY_SERVER): get-deps $(TTY_SERVER_SRC) $(COMMON_SRC)
	go build -o $@ $(TTY_SERVER_SRC)

tty-server/assets_bundle.go: $(TTY_SERVER_ASSETS)
	go-bindata --prefix frontend/public/ -o $@ $^

%.zip: %
	zip $@ $^

frontend:
	cd frontend && npm install && npm run build && cd -

clean:
	rm -fr out/
	rm -fr frontend/public
	@echo "Cleaned"

## Development helper targets
### Runs the server, without TLS/HTTPS (no need for localhost testing)
runs: $(TTY_SERVER)
	$(TTY_SERVER) --web_address :9090 -frontend_path ./frontend/public

test:
	@go test github.com/yi-Tseng/tty-share/testing -v

.PHONY: frontend
