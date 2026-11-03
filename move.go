package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procDiskFree   = kernel32.NewProc("GetDiskFreeSpaceExW")
	procFileAttr   = kernel32.NewProc("GetFileAttributesW")
	fileAttrReparse = 0x400
	invalidAttrs   = uintptr(0xFFFFFFFF)
)

func diskFree(path string) (free, total int64) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var freeAvail, totalBytes, totalFree uint64
	r, _, _ := procDiskFree.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r == 0 {
		return 0, 0
	}
	return int64(freeAvail), int64(totalBytes)
}

func driveRoot(p string) string {
	v := filepath.VolumeName(p)
	if v == "" {
		return "C:\\"
	}
	return v + "\\"
}

func isReparse(p string) bool {
	ptr, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return false
	}
	r, _, _ := procFileAttr.Call(uintptr(unsafe.Pointer(ptr)))
	if r == invalidAttrs {
		return false
	}
	return r&uintptr(fileAttrReparse) != 0
}

func junctionTarget(p string) string {
	t, err := os.Readlink(p)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(t, `\\?\`)
}

// ---- manifest：迁移记录，还原的依据 ----

type ManifestEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Original string `json:"original"`
	Target   string `json:"target"`
	Size     int64  `json:"size"`
	At       string `json:"at"`
}

type Manifest struct {
	Entries []ManifestEntry `json:"entries"`
}

func manifestPath() string { return filepath.Join(exeDir(), "manifest.json") }

func loadManifest() *Manifest {
	m := &Manifest{}
	if data, err := os.ReadFile(manifestPath()); err == nil {
		json.Unmarshal(data, m)
	}
	return m
}

func (m *Manifest) save() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(), data, 0o644)
}

func (m *Manifest) add(e ManifestEntry) {
	for i := range m.Entries {
		if strings.EqualFold(m.Entries[i].Original, e.Original) {
			m.Entries[i] = e
			return
		}
	}
	m.Entries = append(m.Entries, e)
}

func (m *Manifest) remove(original string) {
	out := m.Entries[:0]
	for _, e := range m.Entries {
		if !strings.EqualFold(e.Original, original) {
			out = append(out, e)
		}
	}
	m.Entries = out
}

// ---- 底层动作 ----

func robocopy(src, dst string, move bool) error {
	args := []string{src, dst, "/E", "/XJ", "/R:1", "/W:1", "/NFL", "/NDL", "/NP", "/NJH"}
	if move {
		args = append(args, "/MOVE")
	}
	out, err := runCmd("robocopy", args...)
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return err
		}
	}
	logf("robocopy %q -> %q move=%v exit=%d", src, dst, move, code)
	if code >= 8 {
		logf("robocopy 输出：\n%s", out)
		return fmt.Errorf("robocopy 退出码 %d\n%s", code, tailLines(out, 8))
	}
	return nil
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i := range lines {
		lines[i] = "    " + strings.TrimRight(lines[i], "\r")
	}
	return strings.Join(lines, "\n")
}

func makeJunction(link, target string) error {
	out, err := runCmd("cmd", "/c", "mklink", "/J", link, target)
	logf("mklink /J %q %q err=%v out=%q", link, target, err, strings.TrimSpace(out))
	if err != nil {
		msg := strings.TrimSpace(out)
		hint := ""
		low := strings.ToLower(msg)
		switch {
		case strings.Contains(msg, "已存在"), strings.Contains(low, "already exists"):
			hint = "\n  原因：源目录还占着位置（有文件没搬走，或者程序正在使用它）"
		case strings.Contains(msg, "拒绝访问"), strings.Contains(low, "access is denied"):
			hint = "\n  原因：权限不足，或者该目录被别的进程打开着"
		}
		return fmt.Errorf("%s%s", msg, hint)
	}
	if !isReparse(link) {
		return fmt.Errorf("联接创建后校验失败：%s 不是重解析点", link)
	}
	return nil
}

func removeJunction(link string) error {
	if !isReparse(link) {
		return fmt.Errorf("%s 不是联接，拒绝删除", link)
	}
	out, err := runCmd("cmd", "/c", "rmdir", link)
	logf("rmdir junction %q err=%v out=%q", link, err, strings.TrimSpace(out))
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// pruneEmpty 清掉搬空后的空壳；返回仍然清不掉的残留路径（"" 表示清干净了）
func pruneEmpty(root string) string {
	// 先把文件级残留删掉
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		os.Remove(p)
		return nil
	})
	var dirs []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		if isReparse(d) {
			runCmd("cmd", "/c", "rmdir", d) // 目录联接要按链接删，不能递归
			continue
		}
		os.Remove(d)
	}
	os.Remove(root)
	if _, err := os.Lstat(root); err == nil {
		return root
	}
	return ""
}

// canRename 用"改名再改回来"做最便宜的占用检测
func canRename(p string) bool {
	tmp := p + ".mvprobe"
	if err := os.Rename(p, tmp); err != nil {
		logf("占用检测失败（改名）: %v", err)
		return false
	}
	if err := os.Rename(tmp, p); err != nil {
		logf("占用检测失败（改回）: %v 目录名现在是 %s", err, tmp)
		return false
	}
	return true
}

// ---- 搬运一个目录 ----

func moveItem(cfg Config, it *Item, target string, apply bool) error {
	lp := strings.ToLower(it.Path)
	if strings.Contains(lp, `\appdata\local\packages\`) || strings.Contains(lp, `\windows\`) || strings.Contains(lp, `\program files`) {
		return fmt.Errorf("系统目录不搬：%s", it.Path)
	}

	fmt.Println()
	printBox("迁移计划", []string{
		"来源   " + paint(wht, it.Path),
		"目标   " + paint(wht, target),
		"体积   " + paint(cyn, humanSize(it.Size)) + dimmed(fmt.Sprintf("  %d 个文件", it.Files)),
		"方式   " + paint(mag, "robocopy /MOVE") + dimmed(" → ") + paint(mag, "mklink /J") + dimmed("（对程序透明，无需管理员）"),
	}, 46)
	fmt.Println()

	if !apply {
		printBox("这是演练（dry-run）", []string{
			"什么都没改。真正执行请再跑一次并加 " + paint(yel, "--apply") + " 或交互里确认。",
			"原生等价命令：",
			dimmed(nativeHint(it, cfg.Dest)),
		}, 46)
		return nil
	}

	// 1) 空间校验
	free, _ := diskFree(driveRoot(target))
	if free > 0 && int64(float64(it.Size)*1.05) > free {
		return fmt.Errorf("目标盘剩余 %s，不够放 %s", humanSize(free), humanSize(it.Size))
	}
	ok(fmt.Sprintf("目标盘空间充足（剩余 %s）", humanSize(free)))

	// 2) 目标已被别的应用占着就拒绝（同一个目标夹绝不放两份数据）
	for _, e := range loadManifest().Entries {
		if strings.EqualFold(e.Target, target) && !strings.EqualFold(e.Original, it.Path) {
			return fmt.Errorf("这个位置已经被 %q（原路径 %s）占了，换个目标或先还原它", e.Name, e.Original)
		}
	}

	// 3) 目标已存在则拒绝，避免覆盖
	if st, err := os.Stat(target); err == nil && st.IsDir() {
		if ents, _ := os.ReadDir(target); len(ents) > 0 {
			return fmt.Errorf("目标目录已存在且非空：%s（先改名或删除）", target)
		}
	}
	os.MkdirAll(target, 0o755)

	// 3) 占用检测：能被改名说明没有程序攥着它
	if !canRename(it.Path) {
		return fmt.Errorf("目录正被程序占用（可能有 Agent 还在运行），先退出它再搬：%s", it.Path)
	}
	ok("目录未被占用")

	// 4) 搬运
	sp := startSpinner("正在搬运 " + it.Entry.Name)
	sp.Start()
	err := robocopy(it.Path, target, true)
	sp.Stop()
	if err != nil {
		warn("搬运中断，尽量回滚…")
		robocopy(target, it.Path, true)
		return err
	}
	ok("文件已搬完")

	// 5) 校验体积
	sz, _ := sizeDir(target)
	if it.Size > 0 && sz*100/it.Size < 95 {
		warn(fmt.Sprintf("目标体积 %s 小于源 %s，回滚", humanSize(sz), humanSize(it.Size)))
		robocopy(target, it.Path, true)
		return fmt.Errorf("体积校验不通过")
	}

	// 6) 清空壳；清不掉的（被占用的残留）先改名让位，好让联接能建起来
	leftover := pruneEmpty(it.Path)
	if leftover != "" {
		side := leftover + ".leftover-" + time.Now().Format("20060102-150405")
		if err := os.Rename(leftover, side); err != nil {
			warn("源目录还有东西清不掉（多半是程序正在使用）")
			robocopy(target, it.Path, true)
			return fmt.Errorf("清理源目录失败：%v", err)
		}
		ok("有残留清不掉，先挪到旁边：" + dimmed(side))
		leftover = side
	}

	// 7) 建目录联接
	if err := makeJunction(it.Path, target); err != nil {
		warn("建联接失败，把文件搬回原处…")
		if leftover != "" {
			os.Rename(leftover, it.Path)
		}
		robocopy(target, it.Path, true)
		pruneEmpty(target)
		return err
	}
	ok("目录联接已建立：" + dimmed(it.Path+" → "+target))

	if leftover != "" {
		if err := os.RemoveAll(leftover); err != nil {
			warn("残留删不掉，留在：" + leftover)
		} else {
			ok("残留已清理")
		}
	}

	// 6) 记录
	m := loadManifest()
	m.add(ManifestEntry{
		ID:       it.Entry.ID,
		Name:     it.Entry.Name,
		Original: it.Path,
		Target:   target,
		Size:     sz,
		At:       time.Now().Format("2006-01-02 15:04:05"),
	})
	if err := m.save(); err != nil {
		warn("写迁移记录失败：" + err.Error())
	}
	ok("已写入 manifest.json，可用 \"还原\" 撤回")

	// 刷新 item 状态
	it.Junction = true
	it.Target = target
	it.Size = sz

	printRootTree(cfg)
	return nil
}

// printRootTree 显示"统一根目录 + 分支"的现状
func printRootTree(cfg Config) {
	m := loadManifest()
	if len(m.Entries) == 0 {
		return
	}
	type group struct {
		name  string
		items []ManifestEntry
	}
	var groups []group
	idx := map[string]int{}
	for _, e := range m.Entries {
		branch := "misc"
		if rel, err := filepath.Rel(cfg.Dest, e.Target); err == nil {
			if strings.HasPrefix(rel, "..") {
				branch = "（自定义位置）"
			} else if parts := strings.Split(rel, string(filepath.Separator)); len(parts) > 1 {
				branch = parts[0]
			}
		} else {
			branch = "（自定义位置）"
		}
		if gi, ok := idx[branch]; ok {
			groups[gi].items = append(groups[gi].items, e)
			continue
		}
		idx[branch] = len(groups)
		groups = append(groups, group{branch, []ManifestEntry{e}})
	}

	fmt.Println()
	fmt.Println("  " + paint(gry, "根目录") + " " + paint(wht, cfg.Dest))
	for gi, g := range groups {
		last := gi == len(groups)-1
		conn, pad := "├─", "│  "
		if last {
			conn, pad = "└─", "   "
		}
		var sum int64
		for _, e := range g.items {
			sum += e.Size
		}
		fmt.Printf("  %s %s  %s\n", conn, paint(cyn, g.name),
			dimmed(fmt.Sprintf("%d 项 · %s", len(g.items), humanSize(sum))))
		for ei, e := range g.items {
			c2 := "├─"
			if ei == len(g.items)-1 {
				c2 = "└─"
			}
			fmt.Printf("  %s%s %s %s\n", pad, c2, padRight(e.ID, 24), dimmed(humanSize(e.Size)))
		}
	}
}

func revertItem(cfg Config, e ManifestEntry) error {
	if !isReparse(e.Original) {
		return fmt.Errorf("%s 现在不是联接（可能已手动还原）", e.Original)
	}
	ok("确认是目录联接：" + dimmed(e.Original+" → "+e.Target))

	if err := os.RemoveAll(e.Original); err != nil {
		// 联接用 rmdir 删，RemoveAll 一般也能删；失败再试 rmdir
		exec.Command("cmd", "/c", "rmdir", e.Original).Run()
	}
	if err := os.MkdirAll(filepath.Dir(e.Original), 0o755); err != nil {
		return err
	}

	sp := startSpinner("正在搬回 " + e.Name)
	sp.Start()
	err := robocopy(e.Target, e.Original, true)
	sp.Stop()
	if err != nil {
		return err
	}
	ok("已搬回 " + e.Original)

	os.RemoveAll(e.Target)
	m := loadManifest()
	m.remove(e.Original)
	m.save()
	return nil
}
