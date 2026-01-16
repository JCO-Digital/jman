.PHONY: build dev install clean

build: clean install dist/jman

dist/jman: src/main.ts
	pnpm run build

dev: clean install
	pnpm run dev

install:
	pnpm i

clean:
	rm -rf dist
