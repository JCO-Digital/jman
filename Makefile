.PHONY: build prepare install clean

build: clean prepare bin/jman

bin/jman: src/jman.ts
	bun run build

prepare:
	bun install

install: build
	cp bin/jman ~/.local/bin/

clean:
	rm -rf bin
