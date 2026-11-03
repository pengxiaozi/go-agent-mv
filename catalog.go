package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// CatalogEntry 是一条"已知目录"规则。
// Path 支持 %APPDATA% %LOCALAPPDATA% %USERPROFILE% 变量。
// Suggest: move = 建议搬走并建联接；env = 更适合用环境变量改位置；clean = 只是缓存，可清。
type CatalogEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Suggest  string `json:"suggest"`
	Note     string `json:"note"`
}

var catalog = []CatalogEntry{
	// ---- 桌面端 Agent / Electron ----
	{"claude-desktop", "Claude Desktop", `%APPDATA%\Claude`, "桌面 Agent", "move", "对话历史、附件与缓存"},
	{"claude-3p", "Claude (3p)", `%LOCALAPPDATA%\Claude-3p`, "桌面 Agent", "move", "本地数据目录"},
	{"chatgpt", "ChatGPT Desktop", `%APPDATA%\ChatGPT`, "桌面 Agent", "move", "会话与缓存"},
	{"cursor", "Cursor", `%APPDATA%\Cursor`, "AI 编辑器", "move", "扩展、状态库、缓存"},
	{"cursor-home", "Cursor (~/.cursor)", `%USERPROFILE%\.cursor`, "AI 编辑器", "move", "CLI 与配置"},
	{"windsurf", "Windsurf", `%APPDATA%\Windsurf`, "AI 编辑器", "move", "扩展与缓存"},
	{"trae-cn", "Trae CN", `%APPDATA%\Trae CN`, "AI 编辑器", "move", "国产 AI IDE"},
	{"trae-cn-home", "Trae CN (~/.trae-cn)", `%USERPROFILE%\.trae-cn`, "AI 编辑器", "move", "CLI 数据"},
	{"zcode", "ZCode", `%APPDATA%\ZCode`, "桌面 Agent", "move", "本地数据"},
	{"cherry", "Cherry Studio", `%APPDATA%\CherryStudio`, "桌面 Agent", "move", "多模型客户端"},
	{"chatbox", "Chatbox", `%APPDATA%\xyz.chatboxapp.app`, "桌面 Agent", "move", "会话数据库"},
	{"anythingllm", "AnythingLLM", `%APPDATA%\AnythingLLM`, "桌面 Agent", "move", "向量库与文档"},
	{"lobechat", "LobeChat", `%APPDATA%\lobehub`, "桌面 Agent", "move", "会话与知识库"},
	{"figma-agent", "Figma Agent", `%LOCALAPPDATA%\FigmaAgent`, "桌面 Agent", "move", "本地缓存"},
	{"vscode", "VS Code", `%APPDATA%\Code`, "编辑器", "move", "扩展与缓存（多为插件占用）"},
	{"vscode-ext", "VS Code 扩展", `%USERPROFILE%\.vscode`, "编辑器", "move", "extensions 目录通常最大"},
	{"dsh", "DeepSeek Harness", `%USERPROFILE%\.dsh`, "桌面 Agent", "move", "会话记录与工作区缓存"},

	// ---- 命令行 Agent ----
	{"claude-cli", "Claude Code (~/.claude)", `%USERPROFILE%\.claude`, "CLI Agent", "move", "会话、projects、todo 历史"},
	{"codex", "Codex CLI (~/.codex)", `%USERPROFILE%\.codex`, "CLI Agent", "move", "会话与登录态"},
	{"gemini", "Gemini CLI (~/.gemini)", `%USERPROFILE%\.gemini`, "CLI Agent", "move", "会话与缓存"},
	{"qwen", "Qwen Code (~/.qwen)", `%USERPROFILE%\.qwen`, "CLI Agent", "move", "会话与缓存"},
	{"codebuddy", "CodeBuddy", `%USERPROFILE%\.codebuddy`, "CLI Agent", "move", "国产编码 Agent"},
	{"qoder", "Qoder CN", `%USERPROFILE%\.qoder-cn`, "CLI Agent", "move", "国产编码 Agent"},
	{"copilot", "Copilot (~/.copilot)", `%USERPROFILE%\.copilot`, "CLI Agent", "move", "会话与缓存"},
	{"agents-dir", "Agents (~/.agents)", `%USERPROFILE%\.agents`, "CLI Agent", "move", "多 Agent 共享目录"},
	{"yakit", "Yakit (~/.yakit)", `%USERPROFILE%\.yakit`, "安全工具", "move", "数据库与流量记录"},

	// ---- 模型与缓存 ----
	{"hf-home", "HuggingFace 缓存", `%USERPROFILE%\.cache\huggingface`, "模型缓存", "env", "建议用 HF_HOME 重定向，别硬搬"},
	{"hf-local", "HuggingFace (LocalAppData)", `%LOCALAPPDATA%\huggingface`, "模型缓存", "env", "同上"},
	{"xdg-cache", "通用缓存 ~/.cache", `%USERPROFILE%\.cache`, "模型缓存", "env", "XDG_CACHE_HOME 可整体改盘"},
	{"ollama-models", "Ollama 模型", `%USERPROFILE%\.ollama`, "模型缓存", "env", "OLLAMA_MODELS 改盘更稳"},
	{"ollama-local", "Ollama (LocalAppData)", `%LOCALAPPDATA%\Ollama`, "模型缓存", "move", "程序与日志"},
	{"lmstudio", "LM Studio 模型", `%USERPROFILE%\.lmstudio`, "模型缓存", "move", "models 动辄几十 GB"},
	{"torch", "Torch 模型", `%USERPROFILE%\.cache\torch`, "模型缓存", "env", "TORCH_HOME"},
	{"pip-cache", "pip 缓存", `%LOCALAPPDATA%\pip\Cache`, "包缓存", "env", "PIP_CACHE_DIR"},
	{"paddle", "PaddleOCR / PaddleX", `%USERPROFILE%\.paddlex`, "模型缓存", "move", "OCR 模型仓库"},

	// ---- 系统级（只提示，不建议搬）----
	{"packages", "UWP 应用包数据", `%LOCALAPPDATA%\Packages`, "系统", "clean", "UWP 应用数据，搬了会出问题"},
}

// ---- 配置 ----

type Config struct {
	Dest      string            `json:"dest"`      // 统一根目录，空表示还没设过
	MinSize   int64             `json:"minSize"`   // 自动发现的最小体积阈值
	Layout    string            `json:"layout"`    // branch=根/类别/应用，flat=根/应用
	Overrides map[string]string `json:"overrides"` // 每个应用的自定义目标：id -> 路径
	Inited    bool              `json:"inited"`    // 是否已完成首次设置
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

func configPath() string { return filepath.Join(exeDir(), "config.json") }

func defaultConfig() Config {
	return Config{MinSize: 200 << 20, Layout: "branch", Overrides: map[string]string{}}
}

func loadConfig() Config {
	cfg := defaultConfig()
	if data, err := os.ReadFile(configPath()); err == nil {
		json.Unmarshal(data, &cfg)
	}
	if cfg.Overrides == nil {
		cfg.Overrides = map[string]string{}
	}
	if cfg.Layout == "" {
		cfg.Layout = "branch"
	}
	if cfg.MinSize <= 0 {
		cfg.MinSize = defaultConfig().MinSize
	}
	return cfg
}

// branchFor 把类别映射成根目录下的"分支"文件夹名，保证全是 ASCII、路径安全。
func branchFor(category string) string {
	switch {
	case strings.Contains(category, "包缓存"):
		return "packages"
	case strings.Contains(category, "模型"), strings.Contains(category, "缓存"):
		return "models"
	case strings.Contains(category, "CLI"):
		return "cli"
	case strings.Contains(category, "编辑器"), strings.Contains(category, "IDE"):
		return "ide"
	case strings.Contains(category, "桌面"):
		return "agents"
	case strings.Contains(category, "安全"):
		return "tools"
	case strings.Contains(category, "自动发现"):
		return "other"
	case strings.Contains(category, "系统"):
		return "system"
	}
	return "misc"
}

// targetFor 决定某个应用搬到哪里，优先级：单项自定义 > 结构布局
func targetFor(cfg Config, it *Item) string {
	if p, ok := cfg.Overrides[it.Entry.ID]; ok && p != "" {
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(cfg.Dest, p)
	}
	if cfg.Layout == "flat" {
		return filepath.Join(cfg.Dest, it.Entry.ID)
	}
	return filepath.Join(cfg.Dest, branchFor(it.Entry.Category), it.Entry.ID)
}

func saveConfig(cfg Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath(), data, 0o644)
}
