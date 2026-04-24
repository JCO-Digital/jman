# [4.24.0](https://github.com/JCO-Digital/jman/compare/v4.23.1...v4.24.0) (2026-04-23)


### Features

* **monitor:** add monitorCacheBypass option to bypass frontend caches ([3322275](https://github.com/JCO-Digital/jman/commit/3322275d8c2d7f4128c6637fef2fe44ec07ba744))



## [4.23.1](https://github.com/JCO-Digital/jman/compare/v4.23.0...v4.23.1) (2026-04-23)


### Bug Fixes

* **db:** enforce case-insensitive domain handling ([0e3f8f5](https://github.com/JCO-Digital/jman/commit/0e3f8f53ce82c72f9799a7f30efa153eec8e160a))



# [4.23.0](https://github.com/JCO-Digital/jman/compare/v4.22.0...v4.23.0) (2026-04-23)


### Bug Fixes

* **db:** limit SQLite connections and serialize writes to avoid ([84fdf62](https://github.com/JCO-Digital/jman/commit/84fdf62ba73d24b38d747a6993d87d8417a4bd24))
* **monitor:** add synchronization and in-flight tracking for site checks ([8934a61](https://github.com/JCO-Digital/jman/commit/8934a61f53955836699d8e328b5c4282b33440ba))
* **monitor:** Log error on failed slack send. ([2a4f4ea](https://github.com/JCO-Digital/jman/commit/2a4f4ea21936e96d1ca181e8315ae8dbef6d0432))
* **monitor:** normalize mode to Alert for sites marked down on load ([abeddae](https://github.com/JCO-Digital/jman/commit/abeddae797e82444bec0fa5e871f9abfafe8fef8))


### Features

* **monitor:** add stateful monitoring engine, scheduler, and systemd ([c1ef84e](https://github.com/JCO-Digital/jman/commit/c1ef84e5d92068d40bf48c863004ac4d5dec2287))
* **monitor:** notify Slack when ignoring a site in alert mode ([15d36c5](https://github.com/JCO-Digital/jman/commit/15d36c549f3f76b6dfeb5257113be57466d74800))



# [4.22.0](https://github.com/JCO-Digital/jman/compare/v4.21.1...v4.22.0) (2026-04-21)


### Bug Fixes

* **monitor:** return pending status for unchecked sites and clean up ([869563c](https://github.com/JCO-Digital/jman/commit/869563c617a63bfa53baea1fc6084b47df563302))


### Features

* **monitor:** add DB-backed monitoring API, ignore list, and CLI ([30624b3](https://github.com/JCO-Digital/jman/commit/30624b3af275a54f75f95c0c75f7e4a7a3594985))



## [4.21.1](https://github.com/JCO-Digital/jman/compare/v4.21.0...v4.21.1) (2026-04-20)


### Bug Fixes

* **update:** treat empty response as yes and show [Y/n] prompts ([77b3d75](https://github.com/JCO-Digital/jman/commit/77b3d756335a83788d33a696722f28ff1982b4d9))



