package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type Item struct {
	Entry    CatalogEntry
	Path     string
	Size     int64
	Files    int64
	Junction bool
	Target   string
	Auto     bool
}

var pathVars = strings.NewReplacer(
	"%USERPROFILE%", os.Getenv("USERPROFILE"),
	"%APPDATA%", os.Getenv("APPDATA"),
	"%LOCALAPPDATA%", os.Getenv("LOCALAPPDATA"),
	"%SYSTEMDRIVE%", os.Getenv("SystemDrive"),
)

func expandPath(p string) string {
	return pathVars.Replace(p)
}

// 自动发现用的关键词：只认"像 Agent 的"目录，避免把 .nuget 之类也算进来。
var autoKeywords = regexp.MustCompile(`(?i)(agent|claude|gpt|chatgpt|cursor|ollama|lm-?studio|hugging|comfy|stable-?diffusion|cherry|chatbox|lobe|anythingllm|open-?webui|dify|windsurf|trae|codex|gemini|qwen|dsh|deepseek|codebuddy|copilot|qoder|skillhub|yakit|paddle|antigravity|cc-switch|workbuddy|sheetagent|folo)`)

// 自动发现时忽略的目录（系统或无关）
var autoDeny = map[string]bool{
	"Packages": true, "Temp": true, "Microsoft": true, "Google": true,
	"Programs": true, "ConnectedDevicesPlatform": true, "Comms": true,
	"D3DSCache": true, "CrashDumps": true, "Publishers": true,
}

func sizeDir(root string) (int64, int64) {
	if isReparse(root) {
		if t := junctionTarget(root); t != "" {
			root = t
		}
	}
	var size, files int64
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && isReparse(p) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
			files++
		}
		return nil
	})
	return size, files
}

func scanAll(cfg Config, includeAuto bool, sp *spinner) []*Item {
	seen := map[string]bool{}
	var items []*Item

	add := func(e CatalogEntry, p string, auto bool) {
		key := strings.ToLower(filepath.Clean(p))
		if seen[key] {
			return
		}
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			return
		}
		seen[key] = true
		items = append(items, &Item{Entry: e, Path: p, Auto: auto})
	}

	for _, e := range catalog {
		add(e, expandPath(e.Path), false)
	}

	if includeAuto {
		bases := []string{
			os.Getenv("APPDATA"),
			os.Getenv("LOCALAPPDATA"),
			os.Getenv("USERPROFILE"),
		}
		for _, base := range bases {
			ents, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, de := range ents {
				if !de.IsDir() {
					continue
				}
				name := de.Name()
				if autoDeny[name] {
					continue
				}
				if !autoKeywords.MatchString(name) {
					continue
				}
				p := filepath.Join(base, name)
				fake := CatalogEntry{
					ID:       "auto-" + slug(name),
					Name:     name,
					Path:     p,
					Category: "自动发现",
					Suggest:  "move",
					Note:     "按名称猜的 Agent 目录，自行判断",
				}
				if strings.HasPrefix(name, ".") {
					fake.Category = "自动发现 · 用户目录"
				}
				add(fake, p, true)
			}
		}
	}

	// 并发统计体积
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, it := range items {
		if isReparse(it.Path) {
			it.Junction = true
			it.Target = junctionTarget(it.Path)
			if it.Target != "" {
				it.Size, it.Files = sizeDir(it.Target)
			}
			sp.Add(1)
			continue
		}
		wg.Add(1)
		go func(it *Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			it.Size, it.Files = sizeDir(it.Path)
			sp.Add(1)
		}(it)
	}
	wg.Wait()

	out := items[:0]
	for _, it := range items {
		if it.Auto && it.Size < cfg.MinSize && !it.Junction {
			continue
		}
		if it.Size == 0 && !it.Junction {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })

	// 同名的自动发现项加序号（大的先拿干净名字），保证一个应用一个目标文件夹，绝不混住
	used := map[string]bool{}
	for _, it := range out {
		if !it.Auto {
			used[it.Entry.ID] = true
		}
	}
	for _, it := range out {
		if !it.Auto {
			continue
		}
		base := it.Entry.ID
		if !used[base] {
			used[base] = true
			continue
		}
		for i := 2; ; i++ {
			cand := fmt.Sprintf("%s-%d", base, i)
			if !used[cand] {
				it.Entry.ID = cand
				used[cand] = true
				break
			}
		}
	}

	// 父子重叠时只留父目录（例如 ~/.cache 与 ~/.cache/huggingface）
	final := out[:0]
	for _, it := range out {
		nested := false
		for _, o := range final {
			if isSubPath(o.Path, it.Path) && o.Size >= it.Size {
				nested = true
				break
			}
		}
		if !nested {
			final = append(final, it)
		}
	}
	return final
}

func isSubPath(parent, child string) bool {
	p := strings.ToLower(filepath.Clean(parent))
	c := strings.ToLower(filepath.Clean(child))
	return c != p && strings.HasPrefix(c, p+"\\")
}

// slug 把目录名变成安全的文件夹名：去掉前导 . - @，其余非字母数字转 -，压缩连字符
func slug(s string) string {
	s = strings.TrimLeft(s, ".-_@ ")
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		out = "dir"
	}
	return out
}

func totalSize(items []*Item) int64 {
	var t int64
	for _, it := range items {
		t += it.Size
	}
	return t
}

func suggestLabel(s string) string {
	switch s {
	case "move":
		return "可迁移"
	case "env":
		return "改环境变量"
	case "clean":
		return "仅清理"
	}
	return s
}

// 建议命令：给不想用本工具的人一条可复制的原生命令
func nativeHint(it *Item, dest string) string {
	target := filepath.Join(dest, it.Entry.ID)
	return "robocopy \"" + it.Path + "\" \"" + target + "\" /E /MOVE /XJ  &&  mklink /J \"" + it.Path + "\" \"" + target + "\""
}
