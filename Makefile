.PHONY: build dev install clean

build: clean install bin/jman

dist/jman: src/main.ts
	pnpm esbuild src/main.ts --bundle --minify --platform=node --outfile=dist/jman

dev: clean install
	pnpm esbuild src/main.ts --bundle --watch  --sourcemap --platform=node --outfile=dist/jman

install:
	pnpm i

clean:
	rm -rf dist
