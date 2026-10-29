// api_gateway.go - 算力中转与规则化
// 2025/12/07 HEAD Revision
// Only for test
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeChatCompletion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	escapedContent, _ := json.Marshal(content)
	fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, string(escapedContent))
}

func writeChatCompletionStream(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	escapedContent, _ := json.Marshal(content)
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", string(escapedContent))
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func incrementKeyUsage(clientKey string) error {
	keyUsageMu.Lock()
	defer keyUsageMu.Unlock()

	used, exists := authKeys.Load(clientKey)
	if !exists {
		return fmt.Errorf("invalid api key")
	}
	newUsed := used.(int) + 1
	authKeys.Store(clientKey, newUsed)
	if _, err := keyDB.Exec(`UPDATE access_keys SET used=?, last_used_at=? WHERE api_key=?`,
		newUsed, time.Now().UTC(), clientKey); err != nil {
		return err
	}
	return recordKeyUsage(clientKey, 0, 0, false)
}

func handleGatewayCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	// 检查主干大模型是否可用
	activePri := getActiveModel("LM", "primary")
	if activePri == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AIBase_Server_Error: 系统未配置完整或无可用主干")
		return
	}

	// 鉴权扣费
	authHeader := r.Header.Get("Authorization")
	clientKey := strings.TrimPrefix(authHeader, "Bearer ")
	if _, exists := authKeys.Load(clientKey); !exists {
		writeJSONError(w, http.StatusUnauthorized, "Invalid API Key")
		return
	}

	// 解析请求体，合并客户传来的 System Prompt，提取 User 提问
	var chatReq ChatRequest
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &chatReq); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if err := incrementKeyUsage(clientKey); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to update key usage")
		return
	}

	userPrompt := ""
	clientSysPrompt := ""
	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			clientSysPrompt += msg.Content + "\n"
		}
		if msg.Role == "user" {
			userPrompt = msg.Content
		}
	}

	// 获取系统的 RAG 滑块挡位
	var ragSlider int
	err := sysDB.QueryRow(`SELECT value FROM system_rules WHERE key='rag_slider'`).Scan(&ragSlider)
	if err != nil {
		ragSlider = 1
	}

	if getActiveModel("Embedding", "") == nil {
		ragSlider = 0
	}

	// 执行 RAG 向量召回
	ragContext := ""
	if ragSlider > 0 {
		chunk, score := searchKnowledge(userPrompt)
		// 严苛过滤：相似度低于 0.4 直接清空
		if score > 0.4 {
			ragContext = chunk
		}
	}

	// 如果是强制 RAG (2档)，且没查到知识库，直接网关截停返回 _UNKNOWN_
	if ragSlider == 2 && ragContext == "" {
		if chatReq.Stream {
			writeChatCompletionStream(w, "_UNKNOWN_")
		} else {
			writeChatCompletion(w, "_UNKNOWN_")
		}
		return
	}

	// 生成底层核弹级强制系统指令
	sysPrompt := buildRAGSystemPrompt(ragSlider, ragContext)

	if strings.TrimSpace(clientSysPrompt) != "" {
		sysPrompt = strings.TrimSpace(clientSysPrompt) + "\n\n--------------------\n" + sysPrompt
	}

	// 转交真实大模型引擎处理
	replyContent, err := callRealLM(sysPrompt, userPrompt, activePri)

	// 格式化输出为标准 OpenAI 格式
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "AIBase_API_Error: "+err.Error())
	} else if chatReq.Stream {
		writeChatCompletionStream(w, replyContent)
	} else {
		writeChatCompletion(w, replyContent)
	}
}
