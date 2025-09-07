.PHONY: build dev install clean

build: clean install bin/jman

bin/jman: src/main.ts
	pnpm run build

dev: clean install
	pnpm run watch

install:
	pnpm i

clean:
	rm -rf bin
