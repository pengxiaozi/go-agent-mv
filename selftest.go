package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type sample struct {
	ID, Name, Category string
	NestedJunction     bool // 目录里再套一个联接（Electron 应用很常见，也是上次建联接失败的元凶）
}

// selfTest 在真实环境跑完整闭环：多个类别分别搬走 → 建联接 → 校验 → 打印根目录树 → 全部还原。
func selfTest(cfg Config) {
	samples := []sample{
		{"selftest-cli", "自检样本 · CLI", "CLI Agent", false},
		{"selftest-model", "自检样本 · 模型", "模型缓存", false},
		{"selftest-agent", "自检样本 · 桌面", "桌面 Agent", true},
	}

	cleanup := func(s sample) {
		root := filepath.Join(os.Getenv("LOCALAPPDATA"), s.ID)
		if isReparse(root) {
			removeJunction(root)
		}
		os.RemoveAll(root)
		it := &Item{Entry: CatalogEntry{ID: s.ID, Category: s.Category}, Path: root}
		os.RemoveAll(targetFor(cfg, it))
	}
	for _, s := range samples {
		cleanup(s)
	}
	m := loadManifest()
	for _, s := range samples {
		m.remove(filepath.Join(os.Getenv("LOCALAPPDATA"), s.ID))
	}
	m.save()

	moved := 0
	for _, s := range samples {
		root := filepath.Join(os.Getenv("LOCALAPPDATA"), s.ID)
		os.MkdirAll(filepath.Join(root, "sub"), 0o755)
		for i := 0; i < 12; i++ {
			os.WriteFile(filepath.Join(root, "sub", fmt.Sprintf("f%02d.bin", i)), make([]byte, 128*1024), 0o644)
		}
		os.WriteFile(filepath.Join(root, "hello.txt"), []byte("agentmover selftest\n"), 0o644)
		if s.NestedJunction {
			inner := filepath.Join(root, "sub", "inner-link")
			os.MkdirAll(filepath.Join(os.Getenv("LOCALAPPDATA"), "selftest-inner"), 0o755)
			os.WriteFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "selftest-inner", "x.txt"), []byte("x"), 0o644)
			if err := makeJunction(inner, filepath.Join(os.Getenv("LOCALAPPDATA"), "selftest-inner")); err != nil {
				warn("准备内嵌联接失败（跳过该用例）：" + err.Error())
			}
		}

		size, files := sizeDir(root)
		it := &Item{Entry: CatalogEntry{ID: s.ID, Name: s.Name, Category: s.Category}, Path: root, Size: size, Files: files}

		if err := moveItem(cfg, it, targetFor(cfg, it), true); err != nil {
			fail(s.Name + " 迁移失败：" + err.Error())
			continue
		}
		if !isReparse(root) {
			fail(s.Name + " 迁移后不是联接")
			continue
		}
		if data, err := os.ReadFile(filepath.Join(root, "hello.txt")); err != nil || !strings.Contains(string(data), "selftest") {
			fail(s.Name + " 透过联接读文件失败")
			continue
		}
		moved++
	}

	fmt.Println()
	fmt.Println("  " + paint(gry, "—— 根目录结构 ——"))
	printRootTree(cfg)

	// 全部还原
	m = loadManifest()
	var entries []ManifestEntry
	for _, e := range m.Entries {
		for _, s := range samples {
			if e.ID == s.ID {
				entries = append(entries, e)
			}
		}
	}
	if len(entries) != moved {
		fail(fmt.Sprintf("manifest 记录 %d 条，实际迁移 %d 条", len(entries), moved))
		return
	}
	for _, e := range entries {
		if err := revertItem(cfg, e); err != nil {
			fail(e.Name + " 还原失败：" + err.Error())
			return
		}
	}

	for _, s := range samples {
		root := filepath.Join(os.Getenv("LOCALAPPDATA"), s.ID)
		if isReparse(root) {
			fail(s.Name + " 还原后仍是联接")
			return
		}
		if _, err := os.Stat(filepath.Join(root, "hello.txt")); err != nil {
			fail(s.Name + " 还原后文件缺失")
			return
		}
		cleanup(s)
	}
	ok(fmt.Sprintf("自检通过：%d 个类别搬走 → 联接可读 → 全部还原 → 样本已清理", moved))
}
