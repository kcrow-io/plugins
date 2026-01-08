package limit

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
)

const (
	v2RssKey   = "anno"
	v2CacheKey = "file"

	v1RssKey   = "rss"
	v1CacheKey = "cache"
)

type memStat struct {
	rss   uint64
	cache uint64
	ratio float64
}

func (m *memStat) String() string {
	return fmt.Sprintf("rss(%d), cache(%d), ratio(%.2f)", m.rss, m.cache, m.ratio)
}

func parseStat(ctx context.Context, statpath string) (*memStat, error) {
	data, err := os.ReadFile(statpath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}
	var (
		cacheKey, rssKey string
		cache, rss       uint64
	)
	if cgroups.IsCgroup2UnifiedMode() {
		cacheKey = v2CacheKey
		rssKey = v2RssKey
	} else {
		cacheKey = v1CacheKey
		rssKey = v1RssKey
	}

	foundCache := false
	foundRss := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if foundCache && foundRss {
			break
		}

		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := fields[0]
		var value uint64
		var err error

		if key == cacheKey && !foundCache {
			value, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", cacheKey, err)
			}
			cache = value
			foundCache = true
			continue
		}

		if key == rssKey && !foundRss {
			value, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", rssKey, err)
			}
			rss = value
			foundRss = true
		}
	}

	// 计算ratio，防止除零
	var ratio float64
	if rss > 0 {
		ratio = float64(cache) / float64(rss)
	}

	return &memStat{
		ratio: ratio,
		cache: cache,
		rss:   rss,
	}, nil

}
