all: dist/pihole-sync

dist/pihole-sync:
	go build \
		-o=./dist/pihole-sync\
		-ldflags="-extldflags=-static" \
		-tags sqlite_omit_load_extension \
		pihole-sync.go

clean:
	rm -rf ./dist