package playlists

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	maxFileBytes = 8 << 20
	maxEntries   = maxItems
)

// Entry is one line of a playlist file: a local path (absolute, resolved
// against the file's folder) or, when Path is empty, the URL as written.
type Entry struct {
	Path     string
	URL      string
	Title    string
	Duration float64
}

func (e Entry) String() string {
	if e.Path != "" {
		return e.Path
	}
	return e.URL
}

// Parse reads an .m3u/.m3u8 or .pls file (format by content). dir is the
// folder relative paths are taken from.
func Parse(data []byte, dir string) ([]Entry, error) {
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("the playlist file is larger than %d MB", maxFileBytes>>20)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[playlist]")) || bytes.HasPrefix(bytes.TrimSpace(data), []byte("[Playlist]")) {
		return parsePLS(data, dir)
	}
	return parseM3U(data, dir)
}

func lines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for sc.Scan() {
		out = append(out, strings.TrimSpace(sc.Text()))
	}
	return out
}

func parseM3U(data []byte, dir string) ([]Entry, error) {
	var out []Entry
	var pending Entry
	for _, line := range lines(data) {
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXTINF:"):
			// #EXTINF:<seconds>[ attrs],<title>
			info, title, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			secs, _, _ := strings.Cut(strings.TrimSpace(info), " ")
			pending.Title = source.Clean(title)
			if d, err := strconv.ParseFloat(secs, 64); err == nil && d > 0 && !math.IsInf(d, 0) {
				pending.Duration = d
			}
		case strings.HasPrefix(line, "#"):
		default:
			if len(out) >= maxEntries {
				return out, fmt.Errorf("more than %d entries", maxEntries)
			}
			e := locate(line, dir)
			e.Title, e.Duration = pending.Title, pending.Duration
			out = append(out, e)
			pending = Entry{}
		}
	}
	return out, nil
}

func parsePLS(data []byte, dir string) ([]Entry, error) {
	entries := map[int]*Entry{}
	get := func(n int) *Entry {
		if entries[n] == nil {
			entries[n] = &Entry{}
		}
		return entries[n]
	}
	for _, line := range lines(data) {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		for _, field := range []string{"file", "title", "length"} {
			num, found := strings.CutPrefix(key, field)
			n, err := strconv.Atoi(num)
			if !found || err != nil || n < 1 || n > maxEntries {
				continue
			}
			e := get(n)
			switch field {
			case "file":
				if val == "" {
					continue
				}
				*e = mergeLocated(locate(val, dir), *e)
			case "title":
				e.Title = source.Clean(val)
			case "length":
				if d, err := strconv.ParseFloat(val, 64); err == nil && d > 0 && !math.IsInf(d, 0) {
					e.Duration = d
				}
			}
		}
	}
	nums := make([]int, 0, len(entries))
	for n, e := range entries {
		if e.Path != "" || e.URL != "" {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	out := make([]Entry, 0, len(nums))
	for _, n := range nums {
		out = append(out, *entries[n])
	}
	return out, nil
}

func mergeLocated(loc, old Entry) Entry {
	loc.Title, loc.Duration = old.Title, old.Duration
	return loc
}

// locate turns a playlist line into a clean absolute path, or keeps it as a
// URL when it is one (http streams, anything not file://).
func locate(s, dir string) Entry {
	if u, err := url.Parse(s); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		if u.Scheme == "file" && u.Path != "" {
			return Entry{Path: filepath.Clean(u.Path)}
		}
		return Entry{URL: source.Clean(s)}
	}
	p := filepath.FromSlash(strings.ReplaceAll(s, `\`, "/"))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return Entry{Path: filepath.Clean(p)}
}

// FormatM3U writes an extended M3U (UTF-8) playlist.
func FormatM3U(entries []Entry) []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "#EXTINF:%d,%s\n%s\n", int(math.Round(e.Duration)), oneLine(e.Title), e.Path)
	}
	return []byte(b.String())
}

// FormatPLS writes a PLS version 2 playlist.
func FormatPLS(entries []Entry) []byte {
	var b strings.Builder
	b.WriteString("[playlist]\n")
	for i, e := range entries {
		n := i + 1
		fmt.Fprintf(&b, "File%d=%s\nTitle%d=%s\nLength%d=%d\n", n, e.Path, n, oneLine(e.Title), n, int(math.Round(e.Duration)))
	}
	fmt.Fprintf(&b, "NumberOfEntries=%d\nVersion=2\n", len(entries))
	return []byte(b.String())
}

func oneLine(s string) string { return strings.NewReplacer("\n", " ", "\r", " ").Replace(s) }
