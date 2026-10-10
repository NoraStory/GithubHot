package github

import (
	"testing"
)

// evalRepo 标注样本：真实知名仓库的 topics/描述摘要（截取自各自的 GitHub 描述）。
// 正例 = AI 项目，负例 = 非 AI（含"名字带 AI/gpt 但本体不是 AI"的诱饵，批评 3 指出的误判模式）。
type evalRepo struct {
	name        string
	topics      []string
	description string
	ai          bool
}

var evalRepos = []evalRepo{
	// ---------- 正例：AI 项目 ----------
	{"huggingface/transformers", []string{"transformers", "nlp", "deep-learning"}, "State-of-the-art Machine Learning for AI", true},
	{"ollama/ollama", []string{"llm"}, "Get up and running with large language models locally", true},
	{"langchain-ai/langchain", []string{"llm", "ai-agents", "rag"}, "Building applications with LLMs through composability", true},
	{"openai/openai-python", []string{"openai"}, "The OpenAI Python library provides convenient access to the OpenAI REST API", true},
	{"ggerganov/llama.cpp", []string{"llama", "llm"}, "LLM inference in C/C++", true},
	{"AUTOMATIC1111/stable-diffusion-webui", []string{"stable-diffusion", "image-generation"}, "Stable Diffusion web UI", true},
	{"mckaywrigley/chatbot-ui", []string{"chatbot", "llm"}, "A chatbot UI for local models", true},
	{"microsoft/DeepSpeed", []string{"deep-learning", "machine-learning"}, "Deep learning optimization library for training large models", true},
	{"vllm-project/vllm", []string{"llm"}, "A high-throughput and memory-efficient inference engine for LLMs", true},
	{"StanfordVL/agentbench", []string{"ai-agents", "llm"}, "A comprehensive benchmark for LLM agents", true},
	{"coqui-ai/TTS", []string{"text-to-speech", "speech-recognition"}, "A deep learning toolkit for Text-to-Speech", true},
	{"rerun-io/rerun", []string{"computer-vision", "machine-learning"}, "Visualize streams of robotics and computer vision data", true},
	{"QwenLM/Qwen", []string{"llm", "transformers"}, "The official repo of Qwen large language model series", true},
	{"OpenDevin/OpenDevin", []string{"ai-agents", "llm"}, "An autonomous AI agent for software development", true},
	{"openai/whisper", []string{"speech-recognition", "pytorch"}, "Robust Speech Recognition via Large-Scale Weak Supervision", true},
	{"Runa Capital/awesome-rag", []string{"rag"}, "Curated list of retrieval-augmented generation resources", true},

	// ---------- 负例：非 AI（含诱饵） ----------
	{"curl/curl", []string{"http", "curl"}, "A command line tool and library for transferring data with URL syntax", false},
	{"redis/redis", []string{"database", "cache"}, "Redis is an in-memory database that persists on disk", false},
	{"facebook/react", []string{"react", "frontend"}, "The library for web and native user interfaces", false},
	{"torvalds/linux", []string{"kernel", "c"}, "Linux kernel source tree", false},
	{"gin-gonic/gin", []string{"go", "web"}, "Gin is a HTTP web framework written in Go", false},
	{"gptfdisk/gptfdisk", []string{"partition"}, "GPT fdisk is a partitioning tool for GUID Partition Table disks", false},
	{"gemini-protocol/gemini-cli", []string{"gemini", "protocol"}, "A client for the Gemini protocol (not the AI)", false},
	{"docker/compose", []string{"docker", "containers"}, "Define and run multi-container applications with Docker", false},
	{"obsproject/obs-studio", []string{"streaming", "video"}, "Free and open source software for live streaming and recording", false},
	{"prometheus/prometheus", []string{"monitoring", "metrics"}, "The Prometheus monitoring system and time series database", false},
	{"ziglang/zig", []string{"zig", "compiler"}, "General-purpose programming language and toolchain", false},
	{"junegunn/fzf", []string{"cli", "fuzzy"}, "A command-line fuzzy finder", false},
	{"rustdesk/rustdesk", []string{"remote-desktop"}, "An open-source remote desktop application", false},
	{"syncthing/syncthing", []string{"sync", "p2p"}, "Open Source Continuous File Synchronization", false},
	{"AIDotNet/fast-dotnet", []string{"dotnet"}, "Fast .NET web framework — the 'AI' here is just the org name", false},
	{"skywind3000/kcp", []string{"network", "udp"}, "A Fast and Reliable ARQ Protocol — no relation to ai", false},
	{"immich-app/immich", []string{"photos", "self-hosted"}, "High performance self-hosted photo and video backup solution", false},
	{"Files-community/Files", []string{"file-manager"}, "A modern file manager for Windows", false},
}

// TestAIEval 标注回归：精确率与召回率 ≥ 0.9 才算合格（改关键词/权重必须过此关）。
// 失败时打印混淆明细，让调参有数字可看。
func TestAIEval(t *testing.T) {
	tp, fp, fn := 0, 0, 0
	for _, r := range evalRepos {
		got := IsAI(r.topics, r.description)
		if got == r.ai {
			if r.ai {
				tp++
			}
			continue
		}
		if r.ai {
			fn++
			t.Errorf("[漏判] %s: score=%.2f", r.name, AIScore(r.topics, r.description))
		} else {
			fp++
			t.Errorf("[误判] %s: score=%.2f", r.name, AIScore(r.topics, r.description))
		}
	}
	precision := float64(tp) / float64(tp+fp)
	recall := float64(tp) / float64(tp+fn)
	t.Logf("评估：样本 %d（正 %d / 负 %d）精确率 %.2f 召回率 %.2f（TP=%d FP=%d FN=%d）",
		len(evalRepos), tp+fn, len(evalRepos)-tp-fn, precision, recall, tp, fp, fn)
	if precision < 0.9 {
		t.Fatalf("精确率 %.2f < 0.9", precision)
	}
	if recall < 0.9 {
		t.Fatalf("召回率 %.2f < 0.9", recall)
	}
}

// TestAIScoreSignals 计分语义：topic 单命中即达线；描述单关键词不达线、
// 三个不同关键词达线；普通英文里的 ai/llm 字样不误报。
func TestAIScoreSignals(t *testing.T) {
	cases := []struct {
		name   string
		topics []string
		desc   string
		want   bool
	}{
		{"单 topic 达线", []string{"llm"}, "", true},
		{"两 topic 封顶", []string{"llm", "ai"}, "anything", true},
		{"描述单关键词不达线", []string{}, "a tool for gpt users", false},
		{"描述三关键词达线", []string{}, "build ai agents with llm and rag", true},
		{"普通英文不误报", []string{}, "maintain code with said library", false},
		{"空输入", []string{}, "", false},
	}
	for _, c := range cases {
		if got := IsAI(c.topics, c.desc); got != c.want {
			t.Errorf("%s: want %v got %v (score=%.2f)", c.name, c.want, got, AIScore(c.topics, c.desc))
		}
	}
}

// TestAIScoreBounded 分数有界：超多命中不越界。
func TestAIScoreBounded(t *testing.T) {
	many := []string{"ai", "llm", "rag", "nlp", "transformers", "mlops", "chatbot"}
	if s := AIScore(many, "llm gpt openai machine learning deep learning"); s > 1 {
		t.Fatalf("分数应封顶 1.0，实际 %.2f", s)
	}
	if s := AIScore(nil, "llm gpt openai machine learning deep learning neural network chatbot agentic"); s > aiKeywordCap {
		t.Fatalf("纯描述分数应封顶 %.1f，实际 %.2f", aiKeywordCap, s)
	}
}
