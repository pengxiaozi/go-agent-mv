package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const version = "1.2.1"

// 编译时用 -ldflags "-X main.buildTime=..." 覆盖
var buildTime = "dev"

var lastScan []*Item

func main() {
	initUI()
	cfg := loadConfig()
	var rest []string
	apply := false
	includeAuto := true
	targetFlag := ""

	for _, a := range os.Args[1:] {
		switch {
		case a == "--apply" || a == "-y":
			apply = true
		case a == "--all":
			includeAuto = true
		case a == "--core":
			includeAuto = false
		case strings.HasPrefix(a, "--dest="):
			cfg.Dest = strings.TrimPrefix(a, "--dest=")
		case strings.HasPrefix(a, "--target="):
			targetFlag = strings.TrimPrefix(a, "--target=")
		case a == "-h" || a == "--help":
			printHelp()
			return
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) > 0 && (rest[0] == "help" || rest[0] == "version" || rest[0] == "exec") {
		// 不需要目标目录的命令
	} else {
		ensureDest(&cfg)
	}

	if len(rest) == 0 {
		runMenu(cfg, apply)
		return
	}

	switch rest[0] {
	case "scan":
		cmdScan(cfg, includeAuto)
	case "move":
		items := cmdScan(cfg, includeAuto)
		pick := ""
		if len(rest) > 1 {
			pick = rest[1]
		}
		cmdMove(cfg, items, pick, apply, targetFlag)
	case "revert":
		cmdRevert(cfg, apply)
	case "env":
		cmdEnv(apply)
	case "clean":
		cmdClean(cfg, includeAuto, apply)
	case "report":
		items := cmdScan(cfg, includeAuto)
		p, err := writeReport(cfg, items)
		if err != nil {
			fail(err.Error())
			return
		}
		ok("报告已生成：" + p)
		openReport(p)
	case "plan":
		items := scanOnly(cfg, includeAuto)
		fmt.Println()
		var rows [][]string
		for _, it := range items {
			branch := branchFor(it.Entry.Category)
			if _, ok := cfg.Overrides[it.Entry.ID]; ok {
				branch = "自定义"
			}
			if cfg.Layout == "flat" {
				branch = "平铺"
			}
			if it.Junction {
				branch = "（已迁移）"
			}
			rows = append(rows, []string{
				truncate(it.Entry.Name, 20), humanSize(it.Size), branch, truncate(targetFor(cfg, it), 52),
			})
		}
		printTable([]string{"名称", "体积", "分支", "迁移后位置"}, rows, []bool{false, true, false, false})
		fmt.Println()
		info("根目录 " + paint(wht, cfg.Dest) + dimmed(fmt.Sprintf("，%d 项共 %s 会落到这里", len(items), humanSize(totalSize(items)))))
		info("这只是计划，什么都没动。确认后跑 " + paint(yel, "agentmover move <编号>"))
	case "target":
		cmdTarget(&cfg, rest[1:])
	case "layout":
		if len(rest) > 1 {
			switch rest[1] {
			case "flat", "branch":
				cfg.Layout = rest[1]
				saveConfig(cfg)
				ok("目录结构：" + rest[1])
			default:
				fail("只能是 branch 或 flat")
			}
			return
		}
		cmdLayout(&cfg)
	case "dest":
		if len(rest) > 1 {
			cfg.Dest = rest[1]
			saveConfig(cfg)
			ok("根目录已设为 " + cfg.Dest)
			return
		}
		cmdDest(&cfg)
	case "selftest":
		selfTest(cfg)
	case "exec":
		if len(rest) < 2 {
			fail("用法：agentmover exec <程序> [参数...]")
			return
		}
		out, err := runCmd(rest[1], rest[2:]...)
		fmt.Print(out)
		if err != nil {
			fmt.Println("exit:", err)
		}
	case "help":
		printHelp()
	case "version":
		fmt.Printf("agentmover %s  (%s, windows/amd64)\n", version, buildTime)
	default:
		fail("未知命令：" + rest[0])
		printHelp()
	}
}

func printHelp() {
	printBox("用法", []string{
		paint(wht, "agentmover") + dimmed("                       直接进交互菜单"),
		paint(wht, "agentmover scan [--core]") + dimmed("       只体检，不改任何东西"),
		paint(wht, "agentmover plan") + dimmed("                预览每项会落到哪个分支"),
		paint(wht, "agentmover move <编号|id> --apply") + dimmed("  搬走并建目录联接"),
		paint(wht, "agentmover revert --apply") + dimmed("          全部还原回 C 盘"),
		paint(wht, "agentmover env --apply") + dimmed("             写环境变量改缓存位置"),
		paint(wht, "agentmover clean --apply") + dimmed("           删可再生缓存"),
		paint(wht, "agentmover report") + dimmed("                导出 HTML 报告"),
		paint(wht, "agentmover dest D:\\AgentData") + dimmed("     换根目录（会记住）"),
		paint(wht, "agentmover target <id> <路径>") + dimmed("  只给某一项指定位置"),
		paint(wht, "agentmover layout flat|branch") + dimmed("  平铺或按类别分支"),
		"",
		dimmed("--dest=D:\\AgentData  改目标盘   --all 连小目录一起扫"),
	}, 62)
}

func banner(cfg Config) {
	art := []string{
		" █████╗  ██████╗ ███████╗███╗   ██╗████████╗",
		"██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝",
		"███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   ",
		"██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   ",
		"██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   ",
		"╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝   ",
	}
	fmt.Println()
	for _, l := range art {
		fmt.Println("  " + gradient(l, [3]int{110, 231, 183}, [3]int{96, 165, 250}))
	}
	fmt.Println("  " + dimmed("MOVER · Agent 数据搬家 / C 盘瘦身  v"+version))
	fmt.Println()

	free, total := diskFree("C:\\")
	usedPct := 0.0
	if total > 0 {
		usedPct = float64(total-free) / float64(total) * 100
	}
	status := paint(grn, "充裕")
	if usedPct > 85 {
		status = paint(yel, "偏紧")
	}
	if usedPct > 93 {
		status = paint(red, "告急")
	}
	fmt.Printf("  %s %s / %s 已用 %.0f%%  %s\n",
		paint(gry, "C 盘"), paint(wht, humanSize(free)), humanSize(total), usedPct, status)
	freeD, totalD := diskFree(driveRoot(cfg.Dest))
	destLabel := "目标盘"
	if len(cfg.Dest) >= 2 {
		destLabel = cfg.Dest[:2] + " 盘"
	}
	fmt.Printf("  %s %s / %s 空闲      %s\n",
		paint(gry, destLabel), paint(wht, humanSize(freeD)), humanSize(totalD), dimmed("迁移目标"))
	fmt.Printf("  %s %s\n", paint(gry, "根目录"), paint(wht, cfg.Dest)+dimmed("（所有分支都在这里；菜单 7 可改）"))
}

func runMenu(cfg Config, apply bool) {
	banner(cfg)
	for {
		fmt.Println()
		fmt.Println("  " + paint(cyn, "1") + "  体检扫描     " + dimmed("列出所有在 C 盘堆数据的 Agent"))
		fmt.Println("  " + paint(cyn, "2") + "  搬到 D 盘    " + dimmed("robocopy + 目录联接，程序无感"))
		fmt.Println("  " + paint(cyn, "3") + "  还原回 C 盘  " + dimmed("撤销上面的迁移"))
		fmt.Println("  " + paint(cyn, "4") + "  改环境变量   " + dimmed("HF_HOME / OLLAMA_MODELS / XDG_CACHE_HOME"))
		fmt.Println("  " + paint(cyn, "5") + "  清理缓存     " + dimmed("删掉可再生的 Cache / GPUCache / logs"))
		fmt.Println("  " + paint(cyn, "6") + "  导出报告     " + dimmed("生成一个能发人的 report.html"))
		fmt.Println("  " + paint(cyn, "7") + "  换根目录     " + dimmed("统一存放位置，如 D:\\AgentData"))
		fmt.Println("  " + paint(cyn, "8") + "  目录结构     " + dimmed("分支（根/类别/应用）或平铺（根/应用）"))
		fmt.Println("  " + paint(gry, "0") + "  退出")

		choice := prompt("选择", "0")
		switch choice {
		case "1":
			cmdScan(cfg, true)
		case "2":
			cmdMove(cfg, cmdScan(cfg, true), "", apply, "")
		case "3":
			cmdRevert(cfg, apply)
		case "4":
			_ = cmdEnv(apply)
		case "5":
			cmdClean(cfg, true, apply)
		case "7":
			cmdDest(&cfg)
		case "8":
			cmdLayout(&cfg)
		case "6":
			items := cmdScan(cfg, true)
			if p, err := writeReport(cfg, items); err != nil {
				fail(err.Error())
			} else {
				ok("报告已生成：" + p)
				if confirm("现在打开看吗？") {
					openReport(p)
				}
			}
		case "0", "q", "exit":
			fmt.Println()
			info("收工。迁移记录在 manifest.json，随时能还原。")
			return
		default:
			warn("没这个选项")
		}
	}
}

func scanOnly(cfg Config, includeAuto bool) []*Item {
	fmt.Println()
	sp := startSpinner("正在扫描 Agent 数据目录")
	items := scanAll(cfg, includeAuto, sp)
	sp.Stop()
	return items
}

func cmdScan(cfg Config, includeAuto bool) []*Item {
	items := scanOnly(cfg, includeAuto)
	lastScan = items

	if len(items) == 0 {
		warn("没扫到东西，试试 --all")
		return items
	}

	fmt.Println()
	maxSize := int64(1)
	for _, it := range items {
		if it.Size > maxSize {
			maxSize = it.Size
		}
	}
	var rows [][]string
	for i, it := range items {
		status := suggestLabel(it.Entry.Suggest)
		if it.Junction {
			status = "已迁移"
		}
		cat := truncate(it.Entry.Category, 12)
		rows = append(rows, []string{
			strconv.Itoa(i + 1),
			truncate(it.Entry.Name, 22),
			humanSize(it.Size),
			fmt.Sprint(it.Files),
			status,
			cat,
			truncate(it.Path, 46),
		})
	}
	printTable(
		[]string{"#", "名称", "体积", "文件", "建议", "类别", "路径"},
		rows, []bool{true, false, true, true, false, false, false},
	)

	// 前几名横条
	fmt.Println()
	top := items
	if len(top) > 6 {
		top = top[:6]
	}
	for _, it := range top {
		fmt.Printf("  %s %s %s\n",
			padRight(truncate(it.Entry.Name, 20), 20),
			bar(it.Size, maxSize, 22),
			paint(cyn, humanSize(it.Size)))
	}

	free, total := diskFree("C:\\")
	fmt.Println()
	printBox("体检结论", []string{
		fmt.Sprintf("共 %d 个目录，占用 %s", len(items), paint(red, humanSize(totalSize(items)))),
		fmt.Sprintf("C 盘剩余 %s / %s（已用 %.0f%%）", humanSize(free), humanSize(total),
			float64(total-free)/float64(total)*100),
		dimmed("「可迁移」的直接搬；「改环境变量」的优先改变量，硬搬有时会和程序打架。"),
	}, 56)
	return items
}

func cmdMove(cfg Config, items []*Item, pick string, apply bool, targetFlag string) {
	if len(items) == 0 {
		return
	}
	var chosen *Item
	if pick != "" {
		if n, err := strconv.Atoi(pick); err == nil && n >= 1 && n <= len(items) {
			chosen = items[n-1]
		}
		if chosen == nil {
			for _, it := range items {
				if strings.EqualFold(it.Entry.ID, pick) ||
					strings.EqualFold(filepath.Base(it.Path), pick) {
					chosen = it
					break
				}
			}
		}
		if chosen == nil {
			fail("找不到：" + pick)
			return
		}
	} else {
		pick = prompt("要搬第几个？(输入编号)", "")
		n, err := strconv.Atoi(pick)
		if err != nil || n < 1 || n > len(items) {
			warn("编号无效")
			return
		}
		chosen = items[n-1]
	}

	if chosen.Junction {
		warn("这个已经是联接了，不需要再搬：" + dimmed(chosen.Path+" → "+chosen.Target))
		return
	}
	if chosen.Entry.Suggest == "clean" {
		warn("这类是系统/UWP 数据，搬了应用会坏，拒绝执行。")
		return
	}
	if chosen.Entry.Suggest == "env" {
		warn("这一类更适合改环境变量（菜单 4），硬搬可能被程序写坏。")
		if !confirm("仍然要搬 " + chosen.Entry.Name + " 吗？") {
			return
		}
	}
	if chosen.Size < 1<<20 {
		warn("才 " + humanSize(chosen.Size) + "，搬了不划算。")
	}
	// 目标位置：默认按布局算，也可以现场改（改过的会记进 config.json）
	target := targetFor(cfg, chosen)
	switch {
	case targetFlag != "":
		target = filepath.Clean(targetFlag)
		cfg.Overrides[chosen.Entry.ID] = target
		saveConfig(cfg)
		ok("已记住 " + chosen.Entry.ID + " 的自定义位置：" + target)
	case pick == "":
		fmt.Println()
		info("默认位置：" + paint(wht, target))
		p := prompt("换成别的位置？(直接回车用默认)", "")
		if p != "" {
			target = filepath.Clean(p)
			cfg.Overrides[chosen.Entry.ID] = target
			saveConfig(cfg)
			ok("已记住该位置，下次还原/再迁移都按这个来")
		}
	}

	if !apply && !confirm("执行迁移吗？(会真的移动文件)") {
		info("已取消")
		return
	}
	if err := moveItem(cfg, chosen, target, true); err != nil {
		fail(err.Error())
		warn("文件已回滚到原位置，不会留下搬一半的状态")
		info("完整日志：" + logPath())
		return
	}
	saveConfig(cfg)
}

func cmdRevert(cfg Config, apply bool) {
	m := loadManifest()
	if len(m.Entries) == 0 {
		info("manifest.json 里没有迁移记录")
		return
	}
	fmt.Println()
	var rows [][]string
	for i, e := range m.Entries {
		rows = append(rows, []string{strconv.Itoa(i + 1), e.Name, humanSize(e.Size), e.At, e.Target})
	}
	printTable([]string{"#", "名称", "体积", "迁移时间", "现在的位置"}, rows, []bool{true, false, true, false, false})

	sel := prompt("还原哪个？(编号 / all)", "")
	if sel == "" {
		return
	}
	var targets []ManifestEntry
	if strings.EqualFold(sel, "all") || strings.EqualFold(sel, "a") {
		targets = m.Entries
	} else {
		n, err := strconv.Atoi(sel)
		if err != nil || n < 1 || n > len(m.Entries) {
			warn("编号无效")
			return
		}
		targets = []ManifestEntry{m.Entries[n-1]}
	}
	if !apply && !confirm("确认搬回 C 盘？") {
		info("已取消")
		return
	}
	for _, e := range targets {
		if err := revertItem(cfg, e); err != nil {
			fail(e.Name + "：" + err.Error())
		}
	}
}

func cmdClean(cfg Config, includeAuto bool, apply bool) {
	items := cmdScan(cfg, includeAuto)
	fmt.Println()
	sp := startSpinner("正在找可再生的缓存")
	hits := findCaches(items, 4)
	sp.Stop()
	if len(hits) == 0 {
		ok("没找到超过 5MB 的缓存目录，很干净")
		return
	}
	var rows [][]string
	var total int64
	for i, h := range hits {
		total += h.Size
		rows = append(rows, []string{strconv.Itoa(i + 1), truncate(h.Owner, 18), humanSize(h.Size), truncate(h.Path, 60)})
	}
	printTable([]string{"#", "所属", "体积", "缓存目录"}, rows, []bool{true, false, true, false})
	fmt.Println()
	info(fmt.Sprintf("合计 %s 可删（删了程序会自己重建，只是下次启动慢一点）", paint(yel, humanSize(total))))

	if !apply && !confirm("真的删掉这些缓存吗？") {
		info("已取消")
		return
	}
	var freed int64
	for _, h := range hits {
		if err := os.RemoveAll(h.Path); err != nil {
			warn("删不掉：" + h.Path)
			continue
		}
		freed += h.Size
	}
	ok("已释放 " + humanSize(freed))
}

// cmdDest 设置统一根目录（持久化到 config.json）
// ensureDest 首次启动时让用户自己选根目录，不再写死 D:\AgentData
func ensureDest(cfg *Config) {
	if strings.TrimSpace(cfg.Dest) != "" {
		return
	}
	fmt.Println()
	info("第一次使用：先定一个统一存放位置（随时能改，菜单 7）")

	var rows [][]string
	def := ""
	for _, d := range []string{"C", "D", "E", "F", "G", "H"} {
		free, total := diskFree(d + ":\\")
		if total == 0 {
			continue
		}
		mark := ""
		if d != "C" && free > 50<<30 && def == "" {
			def = d + ":\\AgentData"
			mark = "← 推荐"
		}
		rows = append(rows, []string{d + ":", humanSize(free), humanSize(total), mark})
	}
	printTable([]string{"盘符", "可用", "总容量", ""}, rows, []bool{false, true, true, false})
	if def == "" {
		def = "D:\\AgentData"
	}

	for {
		p := prompt("根目录（也可直接输入任意路径）", def)
		p = filepath.Clean(p)
		if len(filepath.VolumeName(p)) == 0 {
			warn("要写完整路径，例如 D:\\AgentData 或 E:\\AI\\agent-data")
			continue
		}
		if free, _ := diskFree(driveRoot(p)); free == 0 {
			fail("这个盘不存在：" + p)
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			fail("建不了目录：" + err.Error())
			continue
		}
		cfg.Dest = p
		cfg.Inited = true
		saveConfig(*cfg)
		ok("根目录：" + paint(wht, p) + dimmed("  （写进 config.json，下次不再问）"))
		return
	}
}

// cmdTarget 给单个应用指定/清除自定义位置
func cmdTarget(cfg *Config, args []string) {
	if len(args) == 0 {
		if len(cfg.Overrides) == 0 {
			info("还没有任何自定义位置")
			return
		}
		var rows [][]string
		for id, p := range cfg.Overrides {
			rows = append(rows, []string{id, p})
		}
		printTable([]string{"应用 id", "自定义位置"}, rows, []bool{false, false})
		return
	}
	id := args[0]
	if len(args) > 1 && (args[1] == "--reset" || args[1] == "-r") {
		delete(cfg.Overrides, id)
		saveConfig(*cfg)
		ok(id + " 已恢复默认布局")
		return
	}
	if len(args) < 2 {
		fail("用法：agentmover target <应用id> <路径>   或   ... --reset")
		return
	}
	p := filepath.Clean(args[1])
	cfg.Overrides[id] = p
	saveConfig(*cfg)
	ok(id + " → " + p)
}

// cmdLayout 切换"分支 / 平铺"两种结构
func cmdLayout(cfg *Config) {
	cur := "分支（根/类别/应用）"
	if cfg.Layout == "flat" {
		cur = "平铺（根/应用）"
	}
	fmt.Println()
	info("当前结构：" + paint(wht, cur))
	p := prompt("1=分支  2=平铺  (回车不改)", "")
	switch p {
	case "1":
		cfg.Layout = "branch"
	case "2":
		cfg.Layout = "flat"
	default:
		return
	}
	saveConfig(*cfg)
	ok("已改为 " + cfg.Layout + "，只影响之后新迁移的；已迁移的不动")
}

func cmdDest(cfg *Config) {
	fmt.Println()
	info("当前根目录：" + paint(wht, cfg.Dest))
	free, total := diskFree(driveRoot(cfg.Dest))
	fmt.Printf("  %s %s 可用 / %s\n", dimmed("该盘"), humanSize(free), humanSize(total))

	p := prompt("新的根目录（回车保持不变）", cfg.Dest)
	if p == "" || p == cfg.Dest {
		info("没改")
		return
	}
	p = filepath.Clean(p)
	if len(filepath.VolumeName(p)) == 0 {
		warn("请写完整路径，例如 D:\\AgentData")
		return
	}
	free2, _ := diskFree(driveRoot(p))
	if free2 == 0 {
		fail("盘符不存在：" + p)
		return
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		fail("建不了目录：" + err.Error())
		return
	}
	cfg.Dest = p
	saveConfig(*cfg)
	ok("根目录已改为 " + paint(wht, p))
	info("之后每次迁移都会落在 " + p + "\\<分支>\\<应用id>，已迁移的不受影响")
}

func openReport(p string) {
	_ = filepath.Base(p)
	openPath(p)
}
