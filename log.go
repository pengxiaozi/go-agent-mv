package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

var logMu sync.Mutex

func logPath() string { return filepath.Join(exeDir(), "agentmover.log") }

// logf 追加写日志，出错不影响主流程
func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// decodeOutput 把子进程输出从 OEM 代码页（中文 Windows 是 GBK）转成 UTF-8。
// 不做这一步，cmd / robocopy 的中文报错会显示成乱码。
func decodeOutput(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if utf8.Valid(b) {
		return string(b)
	}
	k := syscall.NewLazyDLL("kernel32.dll")
	conv := k.NewProc("MultiByteToWideChar")
	const cpOEM = 1 // CP_OEMCP
	n, _, _ := conv.Call(cpOEM, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	buf := make([]uint16, n)
	conv.Call(cpOEM, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&buf[0])), n)
	return syscall.UTF16ToString(buf)
}

// runCmd 跑外部命令，返回已解码的输出
func runCmd(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	var sb strings.Builder
	c.Stdout = &sb
	c.Stderr = &sb
	err := c.Run()
	return decodeOutput([]byte(sb.String())), err
}
