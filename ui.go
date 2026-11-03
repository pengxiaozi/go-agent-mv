package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unsafe"
)

var (
	useColor = true
	stdin    = bufio.NewReader(os.Stdin)
)

// ---- 调色板（真彩色）----

const (
	rst   = "\x1b[0m"
	b     = "\x1b[1m"
	dim   = "\x1b[2m"
	red   = "\x1b[38;2;248;113;113m"
	grn   = "\x1b[38;2;52;211;153m"
	yel   = "\x1b[38;2;250;204;21m"
	blu   = "\x1b[38;2;96;165;250m"
	mag   = "\x1b[38;2;196;181;253m"
	cyn   = "\x1b[38;2;103;232;249m"
	gry   = "\x1b[38;2;120;135;155m"
	wht   = "\x1b[38;2;226;232;240m"
	bgDark = "\x1b[48;2;30;41;59m"
)

func initUI() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		useColor = false
		return
	}
	if !enableVT() {
		useColor = false
	}
}

func enableVT() bool {
	k := syscall.NewLazyDLL("kernel32.dll")
	gs := k.NewProc("GetStdHandle")
	gm := k.NewProc("GetConsoleMode")
	sm := k.NewProc("SetConsoleMode")
	h, _, _ := gs.Call(uintptr(0xFFFFFFF5))
	var mode uint32
	r, _, _ := gm.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return false
	}
	sm.Call(h, uintptr(mode|0x0004))
	return true
}

func paint(c, s string) string {
	if !useColor {
		return s
	}
	return c + s + rst
}

// ---- 宽度与排版（中文占两格）----

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x303E,
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xA960 && r <= 0xA97F,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE10 && r <= 0xFE19,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F,
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r < 32 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if isWide(r) {
		return 2
	}
	return 1
}

func dispWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

func truncate(s string, w int) string {
	if dispWidth(s) <= w {
		return s
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := runeWidth(r)
		if cur+rw > w-1 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	b.WriteString("…")
	return b.String()
}

func padRight(s string, w int) string {
	s = truncate(s, w)
	if n := w - dispWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func padLeft(s string, w int) string {
	s = truncate(s, w)
	if n := w - dispWidth(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

func gradient(s string, from, to [3]int) string {
	if !useColor {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	n := len(rs)
	for i, r := range rs {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		r0 := from[0] + int(float64(to[0]-from[0])*t)
		g0 := from[1] + int(float64(to[1]-from[1])*t)
		b0 := from[2] + int(float64(to[2]-from[2])*t)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", r0, g0, b0, r)
	}
	b.WriteString(rst)
	return b.String()
}

// ---- 组件 ----

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func printBox(title string, lines []string, minWidth int) {
	w := minWidth
	for _, l := range lines {
		if cw := dispWidth(l) + 2; cw > w {
			w = cw
		}
	}
	if title != "" && dispWidth(title)+4 > w {
		w = dispWidth(title) + 4
	}
	var top string
	if title != "" {
		top = "╭─ " + paint(wht, title) + " " + strings.Repeat("─", max(0, w-dispWidth(title)-3)) + "╮"
	} else {
		top = "╭" + strings.Repeat("─", w) + "╮"
	}
	fmt.Println(paint(gry, top))
	for _, l := range lines {
		fmt.Println(paint(gry, "│") + " " + l + strings.Repeat(" ", max(0, w-1-dispWidth(l))) + paint(gry, "│"))
	}
	fmt.Println(paint(gry, "╰"+strings.Repeat("─", w)+"╯"))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printTable(headers []string, rows [][]string, rightAlign []bool) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = dispWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && dispWidth(c) > widths[i] {
				widths[i] = dispWidth(c)
			}
		}
	}
	var hb strings.Builder
	hb.WriteString("  ")
	for i, h := range headers {
		if rightAlign[i] {
			hb.WriteString(paint(gry, padLeft(h, widths[i])))
		} else {
			hb.WriteString(paint(gry, padRight(h, widths[i])))
		}
		hb.WriteString("   ")
	}
	fmt.Println(hb.String())
	segs := make([]string, len(widths))
	for i, w := range widths {
		segs[i] = strings.Repeat("─", w)
	}
	fmt.Println("  " + paint(gry, dimmed(strings.Join(segs, "───"))))
	for _, r := range rows {
		var b strings.Builder
		b.WriteString("  ")
		for i, c := range r {
			if i >= len(widths) {
				break
			}
			if rightAlign[i] {
				b.WriteString(padLeft(c, widths[i]))
			} else {
				b.WriteString(padRight(c, widths[i]))
			}
			b.WriteString("   ")
		}
		fmt.Println(strings.TrimRight(b.String(), " "))
	}
}

func dimmed(s string) string { return paint(dim, s) }

func bar(v, top int64, cells int) string {
	if top <= 0 {
		return paint(gry, strings.Repeat("░", cells))
	}
	frac := float64(v) / float64(top)
	if frac > 1 {
		frac = 1
	}
	full := int(math.Round(frac * float64(cells)))
	if full == 0 && v > 0 {
		full = 1
	}
	c := grn
	switch {
	case frac > 0.66:
		c = red
	case frac > 0.33:
		c = yel
	}
	return paint(c, strings.Repeat("█", full)) + paint(gry, strings.Repeat("░", cells-full))
}

// ---- 交互 ----

func prompt(msg, def string) string {
	if useColor {
		fmt.Printf("%s %s ", paint(cyn, "›"), msg)
	} else {
		fmt.Printf("> %s ", msg)
	}
	if def != "" {
		fmt.Printf("%s ", dimmed("["+def+"]"))
	}
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func confirm(msg string) bool {
	ans := strings.ToLower(prompt(msg+" (y/N)", "n"))
	return ans == "y" || ans == "yes" || ans == "是"
}

func ok(s string)      { fmt.Println("  " + paint(grn, "✔ ") + s) }
func warn(s string)    { fmt.Println("  " + paint(yel, "▲ ") + s) }
func fail(s string)    { fmt.Println("  " + paint(red, "✘ ") + s) }
func info(s string)    { fmt.Println("  " + paint(blu, "• ") + s) }
func step(n int, s string) {
	fmt.Println("  " + paint(mag, fmt.Sprintf("[%d]", n)) + " " + s)
}

// ---- 转圈 ----

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinner struct {
	msg  string
	n    int64
	stop chan struct{}
	done chan struct{}
	mu   sync.Mutex
}

func startSpinner(msg string) *spinner {
	s := &spinner{msg: msg, stop: make(chan struct{}), done: make(chan struct{})}
	if !useColor {
		close(s.done)
		return s
	}
	go func() {
		defer close(s.done)
		i := 0
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				fmt.Print("\r\x1b[K")
				return
			case <-t.C:
				s.mu.Lock()
				msg := s.msg
				s.mu.Unlock()
				c := atomic.LoadInt64(&s.n)
				line := fmt.Sprintf("%s %s %s", paint(cyn, spinFrames[i%len(spinFrames)]), msg, dimmed(fmt.Sprintf("(%d 项)", c)))
				fmt.Print("\r\x1b[K" + line)
				i++
			}
		}
	}()
	return s
}

func (s *spinner) Start()       {}
func (s *spinner) Add(d int64) { atomic.AddInt64(&s.n, d) }

func (s *spinner) SetMsg(m string) {
	s.mu.Lock()
	s.msg = m
	s.mu.Unlock()
}

func (s *spinner) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}
