## [4.21.1](https://github.com/JCO-Digital/jman/compare/v4.21.0...v4.21.1) (2026-04-20)


### Bug Fixes

* **update:** treat empty response as yes and show [Y/n] prompts ([77b3d75](https://github.com/JCO-Digital/jman/commit/77b3d756335a83788d33a696722f28ff1982b4d9))



# [4.21.0](https://github.com/JCO-Digital/jman/compare/v4.20.2...v4.21.0) (2026-04-20)


### Bug Fixes

* **api:** add security headers middleware and add timeouts to HTTP ([dacfa29](https://github.com/JCO-Digital/jman/commit/dacfa293d1560413d208be9dfdfafef8f6918e17))


### Features

* add configurable CORS, HTTP client utils, and SQL identifier ([1b7376c](https://github.com/JCO-Digital/jman/commit/1b7376c54347acf08c528d3ff5ac6e8cb876d5dc))



## [4.20.2](https://github.com/JCO-Digital/jman/compare/v4.20.1...v4.20.2) (2026-04-16)


### Bug Fixes

* set SQLite pragmas and avoid holding lock while sending Slack ([cfa6a66](https://github.com/JCO-Digital/jman/commit/cfa6a6602186d19aeadaea7d9b76bebe6aa73580))



## [4.20.1](https://github.com/JCO-Digital/jman/compare/v4.20.0...v4.20.1) (2026-04-15)


### Bug Fixes

* **fetch:** handle API errors and non-JSON responses when fetching ([99832a2](https://github.com/JCO-Digital/jman/commit/99832a28825a232332481d1d8d7390a2438dc0bb))


### Performance Improvements

* **cache:** reduce concurrency limit for plugin cache refresh to 12 ([21c8b26](https://github.com/JCO-Digital/jman/commit/21c8b26e6b85252c4df10a106d728c600fba8b55))



# [4.20.0](https://github.com/JCO-Digital/jman/compare/v4.19.1...v4.20.0) (2026-04-15)


### Bug Fixes

* **wpcli:** add --force when installing zip plugins ([cef2aed](https://github.com/JCO-Digital/jman/commit/cef2aed1bb5295128a4a379b271896f038ec1f6f))


### Features

* **plugin:** support installing local .zip plugins by uploading to ([a529d1d](https://github.com/JCO-Digital/jman/commit/a529d1dcf57d1c47347a0a620898235c249b5b2f))



