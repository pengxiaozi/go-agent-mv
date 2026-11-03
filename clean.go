package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CacheHit struct {
	Path  string
	Size  int64
	Owner string
}

// Electron / 各类客户端的"可再生"缓存目录名
var cacheNames = map[string]bool{
	"Cache": true, "cache": true, "cache2": true, "GPUCache": true,
	"Code Cache": true, "CachedData": true, "blob_storage": true,
	"CacheStorage": true, "Service Worker": true, "Crashpad": true,
	"Crash Reports": true, "ShaderCache": true, "DawnCache": true,
	"GrShaderCache": true, "GPUCacheNew": true, "component_crx_cache": true,
	"logs": true, "Logs": true, "tmp": true, "Temp": true, "temp": true,
}

func findCaches(items []*Item, maxDepth int) []CacheHit {
	var hits []CacheHit
	for _, it := range items {
		root := it.Path
		if it.Junction && it.Target != "" {
			root = it.Target
		}
		baseDepth := strings.Count(root, `\\`)
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if p != root && strings.Count(p, `\\`)-baseDepth > maxDepth {
				return filepath.SkipDir
			}
			if p == root {
				return nil
			}
			if isReparse(p) {
				return filepath.SkipDir
			}
			if cacheNames[d.Name()] {
				sz, _ := sizeDir(p)
				if sz > 5<<20 {
					hits = append(hits, CacheHit{Path: p, Size: sz, Owner: it.Entry.Name})
				}
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Size > hits[j].Size })
	return hits
}
