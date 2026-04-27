# [4.26.0](https://github.com/JCO-Digital/jman/compare/v4.25.0...v4.26.0) (2026-04-27)


### Bug Fixes

* **commands:** silence cobra usage and add operation validation ([67dec17](https://github.com/JCO-Digital/jman/commit/67dec17002e65a54fda6fdea22b688e5e9a9fd11))


### Features

* **commands:** accept action before target and add shell completion for ([7fa885e](https://github.com/JCO-Digital/jman/commit/7fa885e1413bf19a72a1d9c440990e70cfaa9777))
* **commands:** add shell completion for monitor and wp commands ([48d846b](https://github.com/JCO-Digital/jman/commit/48d846b48bccb76dda4c5f1f98c970fa89159f78))
* **commands:** add shell completion, reorder args, prefer exact site ([603c8cc](https://github.com/JCO-Digital/jman/commit/603c8cca1100d7a556b33de4945a0841e559d7f1))
* **fetch:** add shell completion for fetch command and rename target to ([6ba12bd](https://github.com/JCO-Digital/jman/commit/6ba12bd242596e05c7e50d3b4683fd40fc1f3a7c))
* **plugin:** suggest cached plugin names for subcommand argument ([8604125](https://github.com/JCO-Digital/jman/commit/8604125b082d4da6115fdf0f8dd609e85035b06d))
* **search:** add fast cache-backed site/plugin search and fast cache ([4f0883f](https://github.com/JCO-Digital/jman/commit/4f0883ff7d3820ec8c602d7f7befe0885dfc9370))


### Performance Improvements

* **wp:** cache command dump and add timeout for completions ([c364171](https://github.com/JCO-Digital/jman/commit/c3641718b8d09ac10e27b150db5a2af9aca77fd3))



# [4.25.0](https://github.com/JCO-Digital/jman/compare/v4.24.0...v4.25.0) (2026-04-24)


### Features

* **update:** add signed releases and client-side signature verification ([3b6e8b1](https://github.com/JCO-Digital/jman/commit/3b6e8b15ef6339c76b85df4d90462bbf27d6807c))



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



