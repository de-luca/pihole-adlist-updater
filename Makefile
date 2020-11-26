all: dist/pihole-adlist-updater

dist/pihole-adlist-updater:
	go build \
		-o=./dist/pihole-adlist-updater\
		-ldflags="-extldflags=-static" \
		-tags sqlite_omit_load_extension \
		pihole-adlist-updater.go

fmt:
	gofmt -s -w pihole-adlist-updater.go

lint:
	golint pihole-adlist-updater.go

clean:
	rm -rf ./dist

tools:
	go get -u golang.org/x/lint/golint