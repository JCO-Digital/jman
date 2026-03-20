# [4.12.0](https://github.com/JCO-Digital/jman/compare/v4.11.0...v4.12.0) (2026-03-20)


### Bug Fixes

* **cache:** Improve plugin fetch error reporting ([0edde0a](https://github.com/JCO-Digital/jman/commit/0edde0a3f5e48f1ee84204ac412577419cdb6e9f))
* **plugin:** Correct site name formatting in remove output ([8d885c7](https://github.com/JCO-Digital/jman/commit/8d885c72b5883672bd756e7c93ee689ec7de0b9e))
* **plugin:** Improve error handling for plugin operations ([114b17d](https://github.com/JCO-Digital/jman/commit/114b17d4ff22dea9f2b1524f82ce91e0c34ebe99))
* **wpcli:** Ensure error is returned from RunWP ([64ce754](https://github.com/JCO-Digital/jman/commit/64ce75495ae6ab3158358bef600fe65c4c565ca7))
* **wpcli:** Improve error handling for WP-CLI commands ([f51f99e](https://github.com/JCO-Digital/jman/commit/f51f99ed6fe56c819c704ea3a4d7fa06d4a9c118))


### Features

* **plugin:** Add plugin alias support ([7d2aeda](https://github.com/JCO-Digital/jman/commit/7d2aeda1d87dcc2a586e964dd72a9645f339755b))
* **plugin:** colorize site names in output ([d1d6a52](https://github.com/JCO-Digital/jman/commit/d1d6a528efb8bd9664694ccade36b5f2409615a4))



# [4.11.0](https://github.com/JCO-Digital/jman/compare/v4.10.1...v4.11.0) (2026-03-19)


### Bug Fixes

* **wpcli:** Return structured data from UpdateCore ([b7769c1](https://github.com/JCO-Digital/jman/commit/b7769c1e52dcd0e468eecfbe476c8d51037d1dc6))


### Features

* **wpcli:** Return new version and language from UpdateCore ([a6fc4bb](https://github.com/JCO-Digital/jman/commit/a6fc4bbb2898bb676aa9a3a558db7c258b4b8023))



## [4.10.1](https://github.com/JCO-Digital/jman/compare/v4.10.0...v4.10.1) (2026-03-19)


### Bug Fixes

* **wpcli:** Disable skip in RunWP for plugin actions ([bbededb](https://github.com/JCO-Digital/jman/commit/bbededbe96173cfc436948d8cfd35eb468db278f))



# [4.10.0](https://github.com/JCO-Digital/jman/compare/v4.9.0...v4.10.0) (2026-03-19)


### Bug Fixes

* RunWP better error handling ([c602704](https://github.com/JCO-Digital/jman/commit/c602704d25a279acd50fa4b2067cedb3e3e1273f))
* **verbosity:** Use verbosity.Println for cancelled operation message ([6d6409e](https://github.com/JCO-Digital/jman/commit/6d6409ee05e47508341e4635f91923b56960c70d))
* **wpcli:** Make update regex multiline aware ([6bff80b](https://github.com/JCO-Digital/jman/commit/6bff80bdf5d19497ec5368439c690dfec2fb35b6))
* **wpcli:** Print update core output verbosely ([30c9aac](https://github.com/JCO-Digital/jman/commit/30c9aac071a970ecdd83045ead075c3f60e4e066))
* **wpcli:** Use strings.SplitSeq for error splitting ([13de961](https://github.com/JCO-Digital/jman/commit/13de96132f51d1e68965778e3b6ea83e7ab3b2eb))


### Features

* **core:** Command to check core for updates and to update. ([cde24be](https://github.com/JCO-Digital/jman/commit/cde24beb85d88e4347d527021897e9fc3a10a76b))
* **core:** improve core check, update, and version commands ([31fb77a](https://github.com/JCO-Digital/jman/commit/31fb77a36843aa51bcdfa392cd1784b2db68c9fc))
* **wpcli:** Enhance core update check output ([10a8406](https://github.com/JCO-Digital/jman/commit/10a84068ca6b47348ecb03d72d65d65ed64c14ee))



# [4.9.0](https://github.com/JCO-Digital/jman/compare/v4.8.0...v4.9.0) (2026-03-10)


### Bug Fixes

* **mods:** show status messages at normal verbosity ([22eae0b](https://github.com/JCO-Digital/jman/commit/22eae0bb2ec29cba0b3ed11b680c149e197584ff))


### Features

* **plugin:** add list, update, and remove subcommands ([e892e30](https://github.com/JCO-Digital/jman/commit/e892e30ca34fe87a2dd305c79dd20f9092febb02))
* **plugin:** batch updates and implement removal ([f7557b4](https://github.com/JCO-Digital/jman/commit/f7557b4db390142bc8ccd211222b079819d8bc30))
* **plugin:** improve plugin list output formatting ([f506f78](https://github.com/JCO-Digital/jman/commit/f506f78c6cc1cbffa0aca3c0eb31edaa3ab2a778))
* **search:** allow selecting specific sites by index in results prompt ([42c5c21](https://github.com/JCO-Digital/jman/commit/42c5c212a938d9705cced2037996aa5cbeb2379d))



