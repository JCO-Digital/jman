package scan

import (
	"io/fs"
	"syscall"
)

// statHash returns the stat details of a FileHash for info.
func statHash(info fs.FileInfo) (FileHash, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileHash{}, false
	}
	return FileHash{
		Dev:   uint64(st.Dev),
		Ino:   st.Ino,
		Size:  st.Size,
		Mtime: st.Mtim.Nano(),
		Ctime: st.Ctim.Nano(),
	}, true
}
