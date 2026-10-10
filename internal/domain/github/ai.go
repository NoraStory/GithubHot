// ai.go GitHub 项目 AI 分类器（算法改进 B 批）：双信号合成分。
//
// 设计要点：
//   - 强信号 = 作者自声明的 GitHub topic 命中精选集（每命中 +0.5，封顶 1.0）——
//     作者自己打的标签比描述文本可靠，一个命中即可达收录线
//   - 弱信号 = 描述关键词词边界匹配（每命中 +0.25，封顶 0.5）——纯描述需 ≥2 个
//     **不同**关键词才达线，单命中不计为 AI（防"名字带 AI 的普通工具"误报：
//     批评 3 指出的误判模式）
//   - AI 收录线 = 0.5。分类质量由 ai_eval_test.go 的真实仓库标注回归测试约束
//     （精确率/召回率 ≥ 0.9），改关键词/权重必须过该测试——调参看数字，不凭感觉。
package github

import (
	"regexp"
	"strings"
)

const (
	aiTopicWeight   = 0.5  // 单个精选 topic 命中的贡献
	aiKeywordWeight = 0.25 // 单个描述关键词命中的贡献
	aiTopicCap      = 1.0  // topic 侧贡献上限
	aiKeywordCap    = 0.5  // 描述侧贡献上限（无 topic 时最多恰好达线）
	// AIThreshold AI 收录线。
	AIThreshold = 0.5
)

// aiTopicSet 精选 AI topic 集（小写；GitHub topics 命名空间内这些词基本无歧义）。
var aiTopicSet = map[string]bool{
	"ai": true, "llm": true, "llms": true, "llm-agent": true, "llm-agents": true,
	"ai-agents": true, "agentic-ai": true, "agentic": true,
	"machine-learning": true, "deep-learning": true, "artificial-intelligence": true,
	"neural-network": true, "neural-networks": true,
	"nlp": true, "natural-language-processing": true, "computer-vision": true,
	"generative-ai": true, "genai": true, "aigc": true,
	"rag": true, "retrieval-augmented-generation": true,
	"diffusion": true, "diffusion-models": true, "stable-diffusion": true,
	"transformer": true, "transformers": true,
	"language-model": true, "large-language-model": true, "large-language-models": true,
	"mlops": true, "chatbot": true, "chatbots": true, "rlhf": true,
	"fine-tuning": true, "speech-recognition": true, "text-to-speech": true,
	"image-generation": true, "multimodal": true, "embeddings": true,
	"vector-database": true, "prompt-engineering": true,
	// 模型厂自声明 topic（openai-python / anthropic-sdk 等官方仓库的第一 topic）
	"openai": true, "anthropic": true, "deepseek": true, "qwen": true, "chatglm": true,
}

// aiKeyword 描述关键词：name 做去重键，re 全部词边界匹配（小写匹配）。
type aiKeyword struct {
	name string
	re   *regexp.Regexp
}

// 精选说明：不收 "gemini"（Gemini 协议社区的 client/server 描述会大量误报）；
// "gpt" 用前缀词边界（覆盖 gpt4/gptq 等衍生词；GUID 分区表场景的误报交给评估测试把关）。
var aiKeywordPatterns = []aiKeyword{
	{"llm", regexp.MustCompile(`\bllms?\b`)},
	{"language-model", regexp.MustCompile(`\blarge language model|\blanguage model\b`)},
	{"gpt", regexp.MustCompile(`\bgpt`)},
	{"openai", regexp.MustCompile(`\bopenai\b`)},
	{"anthropic", regexp.MustCompile(`\banthropic\b`)},
	{"claude", regexp.MustCompile(`\bclaude\b`)},
	{"deepseek", regexp.MustCompile(`\bdeepseek\b`)},
	{"llama", regexp.MustCompile(`\bllama\b`)},
	{"mistral", regexp.MustCompile(`\bmistral\b`)},
	{"machine-learning", regexp.MustCompile(`\bmachine learning\b`)},
	{"deep-learning", regexp.MustCompile(`\bdeep learning\b`)},
	{"neural-network", regexp.MustCompile(`\bneural network`)},
	{"diffusion", regexp.MustCompile(`\bdiffusion model|\bdiffusion-`)},
	{"transformer", regexp.MustCompile(`\btransformers?\b`)},
	{"agentic", regexp.MustCompile(`\bagentic\b|\bai agents?\b`)},
	{"generative-ai", regexp.MustCompile(`\bgenerative ai\b`)},
	{"rag", regexp.MustCompile(`\bretrieval-augmented|\brag\b`)},
	{"fine-tuning", regexp.MustCompile(`\bfine-tun|\bfinetun`)},
	{"chatbot", regexp.MustCompile(`\bchatbot|\bchat bot\b`)},
	{"computer-vision", regexp.MustCompile(`\bcomputer vision\b`)},
	{"speech", regexp.MustCompile(`\bspeech recognition\b|\btext-to-speech\b|\bspeech-to-text\b`)},
	{"rlhf", regexp.MustCompile(`\brlhf\b`)},
	{"multimodal", regexp.MustCompile(`\bmultimodal\b`)},
	{"prompt", regexp.MustCompile(`\bprompt engineering\b`)},
}

// AIScore 项目 AI 信号分（0-1）：topics 强信号 + 描述弱信号合成，见文件头注释。
func AIScore(topics []string, description string) float64 {
	topicScore := 0.0
	for _, t := range topics {
		if aiTopicSet[strings.ToLower(strings.TrimSpace(t))] {
			topicScore += aiTopicWeight
		}
	}
	if topicScore > aiTopicCap {
		topicScore = aiTopicCap
	}
	kwScore := 0.0
	if description != "" {
		d := strings.ToLower(description)
		for _, k := range aiKeywordPatterns {
			if kwScore >= aiKeywordCap {
				break
			}
			if k.re.MatchString(d) {
				kwScore += aiKeywordWeight
			}
		}
	}
	score := topicScore + kwScore
	if score > 1 {
		score = 1
	}
	return score
}

// IsAI AIScore 是否达 AI 收录线。
func IsAI(topics []string, description string) bool {
	return AIScore(topics, description) >= AIThreshold
}
