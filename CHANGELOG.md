# [4.16.0](https://github.com/JCO-Digital/jman/compare/v4.15.0...v4.16.0) (2026-03-26)


### Features

* **cache:** Sanitize plugin info on all reads and writes ([424b86c](https://github.com/JCO-Digital/jman/commit/424b86c357872cb7420f6e6f9986fb94052b802a))
* **db:** Add monitor and slack tables and migration ([18dea2e](https://github.com/JCO-Digital/jman/commit/18dea2eff0d2ae802b02caf0d358f3542a99f1b1))
* **db:** Implement robust database schema migration ([c06423c](https://github.com/JCO-Digital/jman/commit/c06423c5e2b2ab165e7608315cd8597d83be8b9f))
* **db:** Introduce SQLite database for plugin information ([bd337b4](https://github.com/JCO-Digital/jman/commit/bd337b4ebdce36f8e9e63676ae168cd66ae301b0))



# [4.15.0](https://github.com/JCO-Digital/jman/compare/v4.14.1...v4.15.0) (2026-03-23)


### Bug Fixes

* **cache:** sanitize plugin metadata by decoding entities and stripping ([1596077](https://github.com/JCO-Digital/jman/commit/1596077a6cc3d0ef02d4b953c57a456406cc4c78))


### Features

* **mods:** Add ability to enable/disable file mods ([4669dd5](https://github.com/JCO-Digital/jman/commit/4669dd545398a657008937fb3beddbf7a10e9284))



## [4.14.1](https://github.com/JCO-Digital/jman/compare/v4.14.0...v4.14.1) (2026-03-23)


### Bug Fixes

* **admin:** Use normal verbosity for user creation messages ([d9219bc](https://github.com/JCO-Digital/jman/commit/d9219bc1416824cf2a2b71527f7d0748dc53fc53))
* **wpcli:** Pass skip parameter to GetPlugins ([230b8c6](https://github.com/JCO-Digital/jman/commit/230b8c65097ebeb73d3f232a79b902f3bddb00f1))



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



