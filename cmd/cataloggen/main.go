// Command cataloggen regenerates ThorSync's GBA/NDS hash catalogue from the
// CC BY-SA 4.0 Libretro Database no-intro metadata files.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/catalog"
)

var sources = []struct{ platform, name string }{
	{"gba", "Nintendo - Game Boy Advance.dat"},
	{"nds", "Nintendo - Nintendo DS.dat"},
}

const libretroRevision = "d2e31bd3e272e4f10f8641ce3ab00e673a7a318e"

var (
	namePattern = regexp.MustCompile(`^\s*name\s+"(.*)"\s*$`)
	crcPattern  = regexp.MustCompile(`\bcrc\s+([0-9A-Fa-f]{8})\b`)
	sha1Pattern = regexp.MustCompile(`\bsha1\s+([0-9A-Fa-f]{40})\b`)
)

func main() {
	client := &http.Client{Timeout: 60 * time.Second}
	entries := []catalog.Entry{}
	for _, source := range sources {
		url := "https://raw.githubusercontent.com/libretro/libretro-database/" + libretroRevision + "/metadat/no-intro/" + strings.ReplaceAll(source.name, " ", "%20")
		response, err := client.Get(url)
		if err != nil {
			panic(err)
		}
		if response.StatusCode != http.StatusOK {
			panic(response.Status)
		}
		parsed, err := parse(response.Body, source.platform)
		response.Body.Close()
		if err != nil {
			panic(err)
		}
		entries = append(entries, parsed...)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Platform != entries[j].Platform {
			return entries[i].Platform < entries[j].Platform
		}
		return entries[i].Title < entries[j].Title
	})
	output := filepath.Join("internal", "catalog", "data", "catalog.json.gz")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		panic(err)
	}
	file, err := os.Create(output)
	if err != nil {
		panic(err)
	}
	zipper, _ := gzip.NewWriterLevel(file, gzip.BestCompression)
	encoder := json.NewEncoder(zipper)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(entries); err != nil {
		panic(err)
	}
	if err = zipper.Close(); err != nil {
		panic(err)
	}
	if err = file.Close(); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d entries to %s\n", len(entries), output)
}

func parse(reader io.Reader, platform string) ([]catalog.Entry, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	inside := false
	title := ""
	result := []catalog.Entry{}
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "game (" {
			inside = true
			title = ""
			continue
		}
		if !inside {
			continue
		}
		if title == "" {
			if match := namePattern.FindStringSubmatch(line); len(match) == 2 {
				title = strings.ReplaceAll(match[1], `\"`, `"`)
				continue
			}
		}
		if strings.Contains(trimmed, "rom (") {
			crc := crcPattern.FindStringSubmatch(line)
			sha := sha1Pattern.FindStringSubmatch(line)
			if title != "" && len(crc) == 2 && len(sha) == 2 {
				result = append(result, catalog.Entry{Platform: platform, CRC32: strings.ToUpper(crc[1]), SHA1: strings.ToUpper(sha[1]), Title: title})
			}
		}
		if trimmed == ")" {
			inside = false
		}
	}
	return result, scanner.Err()
}
