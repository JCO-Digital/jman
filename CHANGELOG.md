# [4.14.0](https://github.com/JCO-Digital/jman/compare/v4.13.0...v4.14.0) (2026-03-22)


### Bug Fixes

* **cache:** Remove Latest field update from WPVuln ([6854d95](https://github.com/JCO-Digital/jman/commit/6854d95c44a0970347067b35faefffd66b6e8c62))
* **cache:** sanitize plugin metadata by decoding entities and stripping ([2593d75](https://github.com/JCO-Digital/jman/commit/2593d75d0244831c1976386ffc5b66fa0921847f))


### Features

* **api:** Refactor response handling and add plugin info endpoint ([1f4e1bb](https://github.com/JCO-Digital/jman/commit/1f4e1bbadc6aac9269c96d7330547de6dce978dd))
* **cache:** Add plugin info caching and fetching ([f6d4003](https://github.com/JCO-Digital/jman/commit/f6d4003329aa4da956bef93e40f5e115da52ba83))
* **cache:** Add version comparison for plugin updates ([0b070b8](https://github.com/JCO-Digital/jman/commit/0b070b89ee329349c378fe04722e35e4e1753259))
* **cache:** Refactor plugin info update logic ([29b37fc](https://github.com/JCO-Digital/jman/commit/29b37fcbe01ead14b871cb939ffe81d293cf7920))



# [4.13.0](https://github.com/JCO-Digital/jman/compare/v4.12.1...v4.13.0) (2026-03-20)


### Features

* **api:** Implement JWT authentication and rate limiting ([5f14d56](https://github.com/JCO-Digital/jman/commit/5f14d56d989f5d38a29757cfa6336740bcd137eb))
* **cmd:** Add CLI tools for user and credential management ([ebcd5c7](https://github.com/JCO-Digital/jman/commit/ebcd5c7136b4c7b61e3e3155aabeedc814842a17))



## [4.12.1](https://github.com/JCO-Digital/jman/compare/v4.12.0...v4.12.1) (2026-03-20)


### Bug Fixes

* **update:** Use AppVersion for current version check ([218093b](https://github.com/JCO-Digital/jman/commit/218093bbb3d10023a10dd566da740a5d0726ae58))



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



