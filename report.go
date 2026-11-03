package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

const reportCSS = `
:root{--bg:#0b1220;--card:#121c2e;--line:#1e2b40;--tx:#e2e8f0;--dim:#7c8ba1;
--ok:#34d399;--warn:#facc15;--red:#f87171;--blue:#60a5fa;}
*{box-sizing:border-box}
body{margin:0;padding:40px 24px;background:radial-gradient(1200px 600px at 20% -10%,#152238,transparent),var(--bg);
color:var(--tx);font:15px/1.6 "Segoe UI","Microsoft YaHei",system-ui,sans-serif}
.wrap{max-width:1080px;margin:0 auto}
h1{font-size:30px;margin:0 0 4px;letter-spacing:.5px;
background:linear-gradient(90deg,#6ee7b7,#60a5fa);-webkit-background-clip:text;background-clip:text;color:transparent}
.sub{color:var(--dim);margin-bottom:28px}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(178px,1fr));gap:12px;margin-bottom:28px}
.card{background:linear-gradient(180deg,#14203380,#0f1727);border:1px solid var(--line);
border-radius:14px;padding:16px 18px}
.card .k{color:var(--dim);font-size:12px;letter-spacing:.08em;text-transform:uppercase}
.card .v{font-size:26px;font-weight:600;margin-top:6px}
.card.hot .v{color:var(--red)}
table{width:100%;border-collapse:collapse;background:var(--card);border:1px solid var(--line);border-radius:14px;overflow:hidden}
th,td{padding:11px 14px;text-align:left;border-bottom:1px solid var(--line);font-size:14px}
th{color:var(--dim);font-weight:500;font-size:12px;letter-spacing:.08em;text-transform:uppercase;background:#0f1826}
tr:last-child td{border-bottom:none}
td.num{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}
.bar{height:8px;border-radius:99px;background:#1b2740;overflow:hidden;min-width:90px}
.bar i{display:block;height:100%;border-radius:99px;background:linear-gradient(90deg,#34d399,#60a5fa)}
.bar i.hot{background:linear-gradient(90deg,#facc15,#f87171)}
.tag{display:inline-block;padding:2px 9px;border-radius:99px;font-size:12px;border:1px solid}
.tag.ok{color:var(--ok);border-color:#34d39955;background:#34d39912}
.tag.warn{color:var(--warn);border-color:#facc1555;background:#facc1512}
.tag.done{color:var(--blue);border-color:#60a5fa55;background:#60a5fa12}
.path{color:var(--dim);font-size:12px;font-family:Consolas,monospace;word-break:break-all}
footer{margin-top:26px;color:var(--dim);font-size:13px}
code{background:#0f1826;border:1px solid var(--line);border-radius:6px;padding:1px 6px;font-family:Consolas,monospace;font-size:13px}
`

func writeReport(cfg Config, items []*Item) (string, error) {
	total := totalSize(items)
	var maxSize int64 = 1
	var junc int
	for _, it := range items {
		if it.Size > maxSize {
			maxSize = it.Size
		}
		if it.Junction {
			junc++
		}
	}
	freeC, capC := diskFree("C:\\")
	freeD, _ := diskFree(driveRoot(cfg.Dest))

	var sb strings.Builder
	sb.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	sb.WriteString("<title>Agent 数据体检报告</title><style>" + reportCSS + "</style></head><body><div class=\"wrap\">")
	sb.WriteString("<h1>Agent 数据体检报告</h1>")
	sb.WriteString(fmt.Sprintf("<div class=\"sub\">生成时间 %s · 目标盘 %s</div>",
		time.Now().Format("2006-01-02 15:04:05"), htmlEscape(cfg.Dest)))

	sb.WriteString("<div class=\"cards\">")
	card := func(k, v, cls string) {
		sb.WriteString(fmt.Sprintf("<div class=\"card %s\"><div class=\"k\">%s</div><div class=\"v\">%s</div></div>", cls, k, v))
	}
	card("已发现目录", fmt.Sprintf("%d 项", len(items)), "")
	card("占用合计", humanSize(total), "hot")
	card("已迁移", fmt.Sprintf("%d 项", junc), "")
	card("C 盘剩余", fmt.Sprintf("%s / %s", humanSize(freeC), humanSize(capC)), "")
	card("目标盘剩余", humanSize(freeD), "")
	sb.WriteString("</div>")

	sb.WriteString("<table><thead><tr><th>名称</th><th>路径</th><th class=\"num\">体积</th><th>占比</th><th>文件</th><th>状态</th></tr></thead><tbody>")
	for _, it := range items {
		pct := float64(it.Size) / float64(maxSize) * 100
		if pct < 1 {
			pct = 1
		}
		cls, label := "ok", "可迁移"
		if it.Junction {
			cls, label = "done", "已迁移"
		} else if it.Entry.Suggest == "env" {
			cls, label = "warn", "改环境变量"
		} else if it.Entry.Suggest == "clean" {
			cls, label = "warn", "别动（系统）"
		}
		hot := ""
		if pct > 60 {
			hot = " hot"
		}
		sb.WriteString(fmt.Sprintf(
			"<tr><td>%s</td><td class=\"path\">%s</td><td class=\"num\">%s</td>"+
				"<td><div class=\"bar\"><i class=\"%s\" style=\"width:%.0f%%\"></i></div></td>"+
				"<td class=\"num\">%d</td><td><span class=\"tag %s\">%s</span></td></tr>",
			htmlEscape(it.Entry.Name), htmlEscape(it.Path), humanSize(it.Size),
			strings.TrimSpace(hot), pct, it.Files, cls, label))
	}
	sb.WriteString("</tbody></table>")

	sb.WriteString(fmt.Sprintf("<footer>合计 %s 可搬走（C 盘剩余 %s，目标盘 %s 剩余 %s）。<br>迁移方式：<code>robocopy /MOVE</code> 搬到目标盘，再用 <code>mklink /J</code> 在原位建目录联接，程序无感知。</footer>",
		humanSize(total), humanSize(freeC), htmlEscape(cfg.Dest), humanSize(freeD)))
	sb.WriteString("</div></body></html>")

	out := filepath.Join(exeDir(), "report.html")
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return out, nil
}

func openPath(p string) {
	exec.Command("cmd", "/c", "start", "", p).Start()
}
