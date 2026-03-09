// Package tui contains shared TUI components for AI Shell.
package tui

import "strings"

// ─── Common shell command list ────────────────────────────────────────────────

// staticCommands is the seed list of well-known bash commands used when there
// is no history yet, or to extend history-based matches.
var staticCommands = []string{
	"ls", "ls -la", "ls -lh", "ls -alh",
	"cd", "cd ~", "cd ..",
	"pwd",
	"cat", "cat /etc/os-release",
	"echo",
	"grep", "grep -r", "grep -rn",
	"find . -name", "find / -name",
	"ps aux", "ps aux | grep",
	"top", "htop",
	"df -h", "du -sh", "du -sh *",
	"free -h",
	"uname -a",
	"whoami", "id", "hostname",
	"history",
	"man",
	"which", "type",
	"export", "env", "printenv",
	"mkdir", "mkdir -p",
	"rm", "rm -rf", "rm -r",
	"cp", "cp -r",
	"mv",
	"touch",
	"chmod", "chmod +x", "chmod 755",
	"chown",
	"ln", "ln -s",
	"tar", "tar -czf", "tar -xzf", "tar -tzf",
	"zip", "unzip",
	"curl", "curl -O", "curl -I",
	"wget",
	"ssh",
	"scp",
	"ping",
	"netstat", "netstat -tuln",
	"ss -tuln",
	"ip a", "ip r",
	"ifconfig",
	"kill", "kill -9", "killall",
	"systemctl status", "systemctl start", "systemctl stop", "systemctl restart", "systemctl enable",
	"journalctl", "journalctl -f", "journalctl -xe",
	"apt", "apt update", "apt upgrade", "apt install", "apt remove", "apt search",
	"apt-get", "apt-get update", "apt-get install",
	"dpkg", "dpkg -l", "dpkg -i",
	"snap install", "snap list",
	"flatpak install", "flatpak list",
	"git status", "git log", "git log --oneline", "git diff", "git add .", "git commit -m", "git push", "git pull", "git clone",
	"docker ps", "docker ps -a", "docker images", "docker run", "docker exec -it",
	"make", "make build", "make test", "make clean",
	"python3", "python3 -m", "python3 -m pip install",
	"pip install", "pip list",
	"go build", "go run", "go test", "go mod tidy",
	"npm install", "npm run", "npm start", "npm test",
	"node",
	"vim", "nano", "code .",
	"less", "more",
	"head", "head -n", "tail", "tail -n", "tail -f",
	"wc -l", "wc -c",
	"sort", "sort -u", "sort -r",
	"uniq", "uniq -c",
	"awk", "sed", "cut", "tr",
	"xargs",
	"tee",
	"diff",
	"patch",
	"source", "source ~/.bashrc", "source ~/.zshrc",
	"alias", "unalias",
	"date", "cal",
	"uptime",
	"lsblk", "lsof", "lsmod",
	"mount", "umount",
	"fdisk -l", "parted",
	"dmesg", "dmesg | tail",
	"strace", "ltrace",
	"nmap",
	"traceroute", "tracepath",
	"nslookup", "dig",
	"openssl",
	"base64",
	"md5sum", "sha256sum",
	"xxd", "hexdump",
	"file",
	"strings",
	"objdump",
	"ldd",
	"watch",
	"screen", "tmux",
	"nohup",
	"jobs", "fg", "bg",
	"disown",
	"exec",
	"exit",
	"clear",
	"reset",
	"sudo", "sudo su", "sudo -i",
	"su",
	"passwd",
	"useradd", "userdel", "usermod",
	"groupadd",
}

// ─── Suggestion engine ────────────────────────────────────────────────────────

const maxSuggestions = 8

// Suggester maintains an input-history ring and provides prefix-based matches.
type Suggester struct {
	history []string // oldest→newest
	histSet map[string]bool
}

// NewSuggester creates a Suggester pre-loaded with the static command list.
func NewSuggester() *Suggester {
	return &Suggester{histSet: make(map[string]bool)}
}

// Push records a command in history (deduped, most-recent-wins).
func (s *Suggester) Push(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	if s.histSet[cmd] {
		// remove old occurrence so the new one goes to the end
		for i, h := range s.history {
			if h == cmd {
				s.history = append(s.history[:i], s.history[i+1:]...)
				break
			}
		}
	}
	s.history = append(s.history, cmd)
	s.histSet[cmd] = true
}

// Match returns up to maxSuggestions candidates for the given prefix.
// History entries are preferred over static commands; duplicates are skipped.
func (s *Suggester) Match(prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	seen := make(map[string]bool)
	var out []string

	// Walk history newest-first
	for i := len(s.history) - 1; i >= 0 && len(out) < maxSuggestions; i-- {
		h := s.history[i]
		if strings.HasPrefix(h, prefix) && !seen[h] {
			out = append(out, h)
			seen[h] = true
		}
	}

	// Fill from static list
	for _, c := range staticCommands {
		if len(out) >= maxSuggestions {
			break
		}
		if strings.HasPrefix(c, prefix) && !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}

	return out
}

// ─── SuggestionState — cursor + accept ───────────────────────────────────────

// SuggestionState tracks the list of current matches and the selected index.
type SuggestionState struct {
	Matches  []string
	Selected int // -1 = none selected
}

// NewSuggestionState creates an unselected state with the given matches.
func NewSuggestionState(matches []string) SuggestionState {
	return SuggestionState{Matches: matches, Selected: -1}
}

// Next moves the selection down, wrapping around.
func (ss *SuggestionState) Next() {
	if len(ss.Matches) == 0 {
		return
	}
	ss.Selected = (ss.Selected + 1) % len(ss.Matches)
}

// Prev moves the selection up, wrapping around.
func (ss *SuggestionState) Prev() {
	if len(ss.Matches) == 0 {
		return
	}
	if ss.Selected <= 0 {
		ss.Selected = len(ss.Matches) - 1
	} else {
		ss.Selected--
	}
}

// Active returns the currently selected suggestion or "" if none.
func (ss SuggestionState) Active() string {
	if ss.Selected < 0 || ss.Selected >= len(ss.Matches) {
		return ""
	}
	return ss.Matches[ss.Selected]
}

// Ghost returns the inline ghost text (suffix after the typed prefix) for the
// first match (used for the dim inline preview when nothing is explicitly
// selected).
func (ss SuggestionState) Ghost(typed string) string {
	if len(ss.Matches) == 0 {
		return ""
	}
	first := ss.Matches[0]
	if strings.HasPrefix(first, typed) {
		return first[len(typed):]
	}
	return ""
}
