package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sampleOutput 是用于验证 schema 生成与结构化输出的局部类型，
// 避免测试依赖 schemas 包，保持 model 包自身的单元边界。
type sampleOutput struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
	OK    bool     `json:"ok"`
}

// TestJSONSchemaFromType 验证反射生成的 JSON Schema 结构正确。
func TestJSONSchemaFromType(t *testing.T) {
	schema, err := jsonSchemaFromType(reflect.TypeOf(sampleOutput{}))
	if err != nil {
		t.Fatalf("jsonSchemaFromType 返回错误: %v", err)
	}

	if schema["type"] != "object" {
		t.Fatalf("根类型应为 object，实际 %v", schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("additionalProperties 应为 false")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties 缺失或类型不符: %#v", schema["properties"])
	}

	// 逐个校验字段类型映射。
	want := map[string]string{
		"name":  "string",
		"count": "integer",
		"tags":  "array",
		"ok":    "boolean",
	}
	for field, wantType := range want {
		node, exists := props[field]
		if !exists {
			t.Fatalf("properties 缺少字段 %q", field)
		}
		gotType := node.(map[string]any)["type"]
		if gotType != wantType {
			t.Fatalf("字段 %q 类型应为 %s，实际 %v", field, wantType, gotType)
		}
	}

	// tags 的元素类型应为 string。
	tagsNode := props["tags"].(map[string]any)
	if items := tagsNode["items"].(map[string]any); items["type"] != "string" {
		t.Fatalf("tags 元素类型应为 string，实际 %v", items["type"])
	}

	// required 应包含全部字段。
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 4 {
		t.Fatalf("required 应包含 4 个字段，实际 %#v", schema["required"])
	}
}

// TestCompleteJSON 验证结构化补全的端到端行为：
// 请求头（Bearer）、请求体（模型 + response_format + system 消息携带 schema）以及响应反序列化。
func TestCompleteJSON(t *testing.T) {
	var gotHeader http.Header
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("content-type", "application/json")
		// content 字段内的 JSON 是「转义后的 JSON 字符串」，解码后即为目标结构。
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"name\":\"x\",\"count\":2,\"tags\":[\"a\"],\"ok\":true}"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := NewDeepSeekClient(DeepSeekConfig{BaseURL: srv.URL, APIKey: "test-key"})

	var out sampleOutput
	if err := c.CompleteJSON(context.Background(), "请输出", &out); err != nil {
		t.Fatalf("CompleteJSON 返回错误: %v", err)
	}

	if out.Name != "x" || out.Count != 2 || len(out.Tags) != 1 || out.Tags[0] != "a" || !out.OK {
		t.Fatalf("反序列化结果不符: %+v", out)
	}

	// 校验请求头：DeepSeek 用 Authorization Bearer，不再携带版本协商头。
	if gotHeader.Get("authorization") != "Bearer test-key" {
		t.Fatalf("authorization 头不符: %q", gotHeader.Get("authorization"))
	}

	// 校验请求体：模型 + response_format=json_object + system/user 两条消息。
	var req chatRequest
	if err := json.Unmarshal(gotBody, &req); err != nil {
		t.Fatalf("解析请求体失败: %v", err)
	}
	if req.Model != "deepseek-v4-pro" {
		t.Fatalf("模型不符: %q", req.Model)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("结构化补全应携带 response_format=json_object，实际 %#v", req.ResponseFormat)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "system" {
		t.Fatalf("结构化补全应有 system + user 两条消息，实际 %#v", req.Messages)
	}
	if req.Messages[1].Role != "user" || !strings.Contains(req.Messages[1].Content, "JSON Schema") {
		t.Fatalf("user 消息应内嵌 JSON Schema 提示，实际 %#v", req.Messages[1])
	}
}

// TestCompleteJSON_InvalidJSON 验证模型输出非法 JSON 时返回 error（触发上层降级）。
func TestCompleteJSON_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"这不是 JSON"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := NewDeepSeekClient(DeepSeekConfig{BaseURL: srv.URL, APIKey: "test-key"})

	var out sampleOutput
	if err := c.CompleteJSON(context.Background(), "请输出", &out); err == nil {
		t.Fatal("模型输出非法 JSON 时应返回 error")
	}
}

// TestComplete_RetriesTransientError 验证 429 等瞬态错误按指数退避重试直至成功。
func TestComplete_RetriesTransientError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	c := NewDeepSeekClient(DeepSeekConfig{BaseURL: srv.URL, APIKey: "k"})
	c.backoff = func(int) time.Duration { return 0 } // 测试注入 0 退避，加速运行

	got, err := c.Complete(context.Background(), "hi")
	if err != nil {
		t.Fatalf("期望重试后成功，实际错误: %v", err)
	}
	if got != "hi" {
		t.Fatalf("结果不符: %q", got)
	}
	if calls != 3 {
		t.Fatalf("期望 3 次调用（2 次失败 + 1 次成功），实际 %d", calls)
	}
}

// TestComplete_NonRetryableReturnsError 验证 4xx 业务错误不重试、直接返回 *APIError。
func TestComplete_NonRetryableReturnsError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"authentication_error"}}`))
	}))
	defer srv.Close()

	c := NewDeepSeekClient(DeepSeekConfig{BaseURL: srv.URL, APIKey: "bad"})
	c.backoff = func(int) time.Duration { return 0 }

	_, err := c.Complete(context.Background(), "hi")
	if err == nil {
		t.Fatal("期望返回错误")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("期望 *APIError，实际 %T", err)
	}
	if apiErr.IsRetryable() {
		t.Fatal("401 不应被判定为可重试")
	}
	if calls != 1 {
		t.Fatalf("非瞬态错误不应重试，期望 1 次调用，实际 %d", calls)
	}
}
