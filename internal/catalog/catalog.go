package catalog

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

type Entry struct {
	Platform string `json:"p"`
	CRC32    string `json:"c"`
	SHA1     string `json:"s"`
	Title    string `json:"t"`
}

//go:embed data/catalog.json.gz
var compressed []byte

var (
	loadOnce sync.Once
	byCRC    map[string]Entry
	bySHA1   map[string]Entry
)

func Lookup(platform, crc32, sha1 string) (Entry, bool) {
	loadOnce.Do(load)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if hash := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(crc32, "0x"))); hash != "" {
		entry, ok := byCRC[platform+":"+hash]
		if ok {
			return entry, true
		}
	}
	if hash := strings.ToUpper(strings.TrimSpace(sha1)); hash != "" {
		entry, ok := bySHA1[platform+":"+hash]
		if ok {
			return entry, true
		}
	}
	return Entry{}, false
}

func load() {
	byCRC = map[string]Entry{}
	bySHA1 = map[string]Entry{}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return
	}
	defer reader.Close()
	var entries []Entry
	if err := json.NewDecoder(reader).Decode(&entries); err != nil {
		return
	}
	for _, entry := range entries {
		if entry.CRC32 != "" {
			byCRC[entry.Platform+":"+entry.CRC32] = entry
		}
		if entry.SHA1 != "" {
			bySHA1[entry.Platform+":"+entry.SHA1] = entry
		}
	}
}
