.PHONY: build dev install clean

build: clean install bin/jman

bin/jman: src/jman.ts
	bun run build

install:
	bun install

clean:
	rm -rf bin
