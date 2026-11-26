//go:build !linux

package scan

import "io/fs"

// statHash has no ctime to go by outside Linux, so no hash is reused.
func statHash(fs.FileInfo) (FileHash, bool) { return FileHash{}, false }
