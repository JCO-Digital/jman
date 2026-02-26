# [4.3.0](https://github.com/JCO-Digital/jman/compare/v4.2.0...v4.3.0) (2026-02-26)


### Bug Fixes

* **cache:** filter out non-WordPress sites from site list ([3b2baa8](https://github.com/JCO-Digital/jman/commit/3b2baa849bfd019870d8cb38e4ef8e309274c878))


### Features

* **api:** add jman-api REST service ([c024dac](https://github.com/JCO-Digital/jman/commit/c024dacde8b84592be8bea94433bb93034f6787c))
* **search:** add plugin search and case-insensitive site matching ([383789c](https://github.com/JCO-Digital/jman/commit/383789c5136603c58601c4cc7fd39dd06b803300))



# [4.2.0](https://github.com/JCO-Digital/jman/compare/v4.1.0...v4.2.0) (2026-02-24)


### Features

* **fetch:** add support for fetching plugin vulnerabilities ([de08dc5](https://github.com/JCO-Digital/jman/commit/de08dc5e3a863634a6bdb80c4347ac0fff093fa6))



# [4.1.0](https://github.com/JCO-Digital/jman/compare/v4.0.1...v4.1.0) (2026-02-24)


### Bug Fixes

* **wpcli:** improve error reporting ([82a887d](https://github.com/JCO-Digital/jman/commit/82a887d3e173bf3c0ab9ce43a632bcae54816c0e))


### Features

* **fetch:** support targeting specific resources for cache update ([4116051](https://github.com/JCO-Digital/jman/commit/41160518b7b4e8023ef7e6e99ce793db84a1ddec))



## [4.0.1](https://github.com/JCO-Digital/jman/compare/v4.0.0...v4.0.1) (2026-02-24)


### Bug Fixes

* **root:** show version only in verbose mode ([f74ab69](https://github.com/JCO-Digital/jman/commit/f74ab690a2d650939086fb6bbb6085b1a9fa65bb))
* separate stdout and stderror to allow piping to files. ([6ee0ebd](https://github.com/JCO-Digital/jman/commit/6ee0ebd4a9501a7e605baa6ad827392e070b080f))
* **update:** require valid download URL for update notification ([4392dbc](https://github.com/JCO-Digital/jman/commit/4392dbcfb3a00494345f258b649562f88d9e68c1))
* **vuln:** fix typo in CVSS score label ([00eb324](https://github.com/JCO-Digital/jman/commit/00eb32485fb8ef82653b7ea131fdf9ad839a6888))
* **vuln:** sanitize HTML tags and entities in reports ([85f4eb8](https://github.com/JCO-Digital/jman/commit/85f4eb8912fd81fbe8e4fd242e0e99812bf9294c))


### Features

* **cmd:** add verbosity flags and level management ([94a59ab](https://github.com/JCO-Digital/jman/commit/94a59abe2e5ead5175de43d046059f3e3ee98ce3))
* **update:** add command to check for latest version ([c93affd](https://github.com/JCO-Digital/jman/commit/c93affd37df1062b7e37360639a54c77e5b1d974))
* **update:** implement automatic self-updating ([473ad52](https://github.com/JCO-Digital/jman/commit/473ad52e3bb8290068e7427351d033e9f0d4d04b))
* **update:** show download progress during updates ([51396b6](https://github.com/JCO-Digital/jman/commit/51396b66e76430be95cede8e6be657e3563eab09))
* **verbosity:** implement verbosity control and conditional printing ([5b375df](https://github.com/JCO-Digital/jman/commit/5b375dfc63c84f9d0a5722df7995ee852047f5a0))


### Performance Improvements

* **cache:** fetch plugins concurrently ([4dbe3b9](https://github.com/JCO-Digital/jman/commit/4dbe3b967612909aac86e2d1994206cd87f6ca41))
* **cache:** limit concurrent plugin fetching to 24 ([560aa8b](https://github.com/JCO-Digital/jman/commit/560aa8b3d659cda046318cd7f684c6abd115c9e0))



