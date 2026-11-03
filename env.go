package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type EnvDef struct {
	Var   string
	Value string
	Why   string
}

var envDefs = []EnvDef{
	{"HF_HOME", `D:\AgentData\huggingface`, "HuggingFace 模型与数据集缓存（最大头）"},
	{"HF_HUB_CACHE", `D:\AgentData\huggingface\hub`, "Hub 下载缓存"},
	{"TRANSFORMERS_CACHE", `D:\AgentData\huggingface\transformers`, "老版本 transformers 认这个变量"},
	{"XDG_CACHE_HOME", `D:\AgentData\cache`, "Linux 风格工具的通用缓存根目录"},
	{"OLLAMA_MODELS", `D:\AgentData\ollama\models`, "Ollama 模型权重"},
	{"PIP_CACHE_DIR", `D:\AgentData\pip-cache`, "pip 下载缓存"},
	{"TORCH_HOME", `D:\AgentData\torch`, "PyTorch 预训练权重"},
	{"UV_CACHE_DIR", `D:\AgentData\uv-cache`, "uv 包缓存（新一代 Python 工具链）"},
}

func readUserEnv(name string) string {
	cmd := exec.Command("reg", "query", `HKCU\Environment`, "/v", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 3 && strings.EqualFold(fields[0], name) {
			return strings.Join(fields[2:], " ")
		}
	}
	return ""
}

func writeUserEnv(name, value string) error {
	cmd := exec.Command("reg", "add", `HKCU\Environment`, "/v", name, "/t", "REG_SZ", "/d", value, "/f")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	os.Setenv(name, value)
	return nil
}

func renderEnv() {
	fmt.Println()
	var rows [][]string
	for _, d := range envDefs {
		cur := readUserEnv(d.Var)
		shown := cur
		if shown == "" {
			shown = "（未设置）"
		}
		rows = append(rows, []string{d.Var, truncate(shown, 40), d.Value, d.Why})
	}
	printTable([]string{"变量", "当前值", "建议值", "用途"}, rows, []bool{false, false, false, false})
}

func cmdEnv(apply bool) error {
	renderEnv()
	fmt.Println()
	if !apply {
		printBox("演练模式", []string{
			"加 " + paint(yel, "--apply") + " 或交互里选 y 才会写注册表。",
			"设置后只对" + paint(wht, "新开的终端/新启动的程序") + "生效，当前进程不受影响。",
		}, 46)
		return nil
	}
	if !confirm("把这些变量写入 HKCU\\Environment 吗？") {
		warn("已取消")
		return nil
	}
	for _, d := range envDefs {
		if err := writeUserEnv(d.Var, d.Value); err != nil {
			fail(d.Var + " 写入失败：" + err.Error())
			continue
		}
		ok(d.Var + " = " + d.Value)
	}
	fmt.Println()
	info("新开一个 PowerShell 窗口再启动 Agent，变量才生效")
	return nil
}
