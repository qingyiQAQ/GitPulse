package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gitpulse/gitpulse/internal/schemas"
)

// searchRespBody 是一段可复用的 Search API 成功响应：
// 第一个项字段齐全，第二个项的 description / topics 为 null，用于覆盖 null 处理。
const searchRespBody = `{
  "items": [
    {"full_name":"avelino/awesome-go","name":"awesome-go","owner":{"login":"avelino"},"description":"curated list","language":"Go","stargazers_count":123,"html_url":"https://github.com/avelino/awesome-go","topics":["golang","awesome"]},
    {"full_name":"torvalds/linux","name":"linux","owner":{"login":"torvalds"},"description":null,"language":"C","stargazers_count":456,"html_url":"https://github.com/torvalds/linux","topics":null}
  ]
}`

// newTestTool 构造一个指向 mock server 的工具：
// baseURL 指向 srv、退避注入为 0（避免真实等待），使测试不发真实网络请求且运行飞快。
//
// 入参：
//   - t：测试对象（用于 helper 标记与自动清理 server）。
//   - token：要注入给工具的 GitHub token。
//   - handler：mock server 的处理函数，可在其中断言请求并返回响应。
//
// 返回值：
//   - *GitHubTool：已指向 mock server 的工具。
func newTestTool(t *testing.T, token string, handler http.HandlerFunc) *GitHubTool {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	g := NewGitHubTool(token)
	g.baseURL = srv.URL
	g.backoff = func(int) time.Duration { return 0 }
	return g
}

// TestFetchTrending 验证热门列表抓取的请求参数与结果映射。
func TestFetchTrending(t *testing.T) {
	var gotAuth, gotQ, gotSort, gotOrder, gotPerPage string
	g := newTestTool(t, "test-token", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		q := r.URL.Query()
		gotQ = q.Get("q")
		gotSort = q.Get("sort")
		gotOrder = q.Get("order")
		gotPerPage = q.Get("per_page")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(searchRespBody))
	})

	repos, err := g.fetchTrending(context.Background(), "Go", 5)
	if err != nil {
		t.Fatalf("fetchTrending 返回错误: %v", err)
	}

	// 校验请求头与查询参数。
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization 头不符: %q", gotAuth)
	}
	if !strings.Contains(gotQ, "created:>") {
		t.Fatalf("q 应包含 created:> 时间过滤，实际 %q", gotQ)
	}
	if !strings.Contains(gotQ, "language:Go") {
		t.Fatalf("q 应包含语言过滤，实际 %q", gotQ)
	}
	if gotSort != "stars" || gotOrder != "desc" || gotPerPage != "5" {
		t.Fatalf("排序参数不符: sort=%q order=%q per_page=%q", gotSort, gotOrder, gotPerPage)
	}

	// 校验结果映射。
	if len(repos) != 2 {
		t.Fatalf("期望 2 个仓库，实际 %d", len(repos))
	}
	first := repos[0]
	if first.ID != "avelino/awesome-go" || first.Owner != "avelino" || first.Name != "awesome-go" ||
		first.Description != "curated list" || first.Language != "Go" || first.Stars != 123 ||
		first.URL != "https://github.com/avelino/awesome-go" || len(first.Topics) != 2 {
		t.Fatalf("首个仓库映射不符: %+v", first)
	}
	// 第二个仓库的 null 字段应映射为「零值」。
	second := repos[1]
	if second.Description != "" || second.Topics != nil {
		t.Fatalf("null 字段应映射为零值，实际 Description=%q Topics=%#v", second.Description, second.Topics)
	}
}

// TestFetchTrending_Defaults 验证 limit<=0 落默认值、language 为空不追加过滤、无 token 不带鉴权头。
func TestFetchTrending_Defaults(t *testing.T) {
	var gotAuth, gotQ, gotPerPage string
	g := newTestTool(t, "", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		q := r.URL.Query()
		gotQ = q.Get("q")
		gotPerPage = q.Get("per_page")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(searchRespBody))
	})

	if _, err := g.fetchTrending(context.Background(), "", 0); err != nil {
		t.Fatalf("fetchTrending 返回错误: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("无 token 时不应携带 Authorization 头，实际 %q", gotAuth)
	}
	if gotPerPage != "20" {
		t.Fatalf("limit<=0 应落到默认 20，实际 per_page=%q", gotPerPage)
	}
	if strings.Contains(gotQ, "language:") {
		t.Fatalf("language 为空时 q 不应含语言过滤，实际 %q", gotQ)
	}
}

// TestFetchDetail 验证单仓库详情抓取的请求路径与结果映射。
func TestFetchDetail(t *testing.T) {
	var gotPath string
	g := newTestTool(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"full_name":"avelino/awesome-go","name":"awesome-go","owner":{"login":"avelino"},"description":"d","language":"Go","stargazers_count":9,"html_url":"https://github.com/avelino/awesome-go","topics":["x"]}`))
	})

	repo, err := g.fetchDetail(context.Background(), "avelino", "awesome-go")
	if err != nil {
		t.Fatalf("fetchDetail 返回错误: %v", err)
	}
	if gotPath != "/repos/avelino/awesome-go" {
		t.Fatalf("请求路径不符: %q", gotPath)
	}
	if repo.ID != "avelino/awesome-go" || repo.Name != "awesome-go" || repo.Stars != 9 {
		t.Fatalf("详情映射不符: %+v", repo)
	}
}

// TestRun_Dispatch 验证 Run 按 action 分发，以及非法参数/未知动作的报错。
func TestRun_Dispatch(t *testing.T) {
	g := newTestTool(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if strings.Contains(r.URL.Path, "search") {
			_, _ = w.Write([]byte(searchRespBody))
			return
		}
		_, _ = w.Write([]byte(`{"full_name":"o/r","name":"r","owner":{"login":"o"}}`))
	})

	// trending：应返回仓库数组。
	raw, err := g.Run(context.Background(), json.RawMessage(`{"action":"trending","limit":3}`))
	if err != nil {
		t.Fatalf("trending 动作返回错误: %v", err)
	}
	var repos []schemas.Repo
	if err := json.Unmarshal(raw, &repos); err != nil {
		t.Fatalf("trending 结果不是合法 JSON 数组: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("trending 应返回 2 个仓库，实际 %d", len(repos))
	}

	// detail：应返回单个仓库。
	raw, err = g.Run(context.Background(), json.RawMessage(`{"action":"detail","owner":"o","name":"r"}`))
	if err != nil {
		t.Fatalf("detail 动作返回错误: %v", err)
	}
	var repo schemas.Repo
	if err := json.Unmarshal(raw, &repo); err != nil {
		t.Fatalf("detail 结果不是合法 JSON 对象: %v", err)
	}
	if repo.Name != "r" {
		t.Fatalf("detail 结果不符: %+v", repo)
	}

	// 未知 action 应报错。
	if _, err := g.Run(context.Background(), json.RawMessage(`{"action":"bogus"}`)); err == nil {
		t.Fatal("未知 action 应返回错误")
	}
	// detail 缺 owner/name 应报错。
	if _, err := g.Run(context.Background(), json.RawMessage(`{"action":"detail"}`)); err == nil {
		t.Fatal("detail 缺 owner/name 应返回错误")
	}
	// 非法 JSON 参数应报错。
	if _, err := g.Run(context.Background(), json.RawMessage(`{bad`)); err == nil {
		t.Fatal("非法 JSON 参数应返回错误")
	}
}

// TestFetchTrending_RetriesTransient 验证 429 等瞬态错误按退避重试直至成功。
func TestFetchTrending_RetriesTransient(t *testing.T) {
	calls := 0
	g := newTestTool(t, "", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			// 不带 Retry-After 头，走退避（已被注入为 0），加速测试。
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limited"}`))
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(searchRespBody))
	})

	repos, err := g.fetchTrending(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("期望重试后成功，实际错误: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("结果数量不符: %d", len(repos))
	}
	if calls != 3 {
		t.Fatalf("期望 3 次调用（2 次失败 + 1 次成功），实际 %d", calls)
	}
}

// TestFetchDetail_NonRetryable 验证 404 等业务错误不重试、直接返回 *githubHTTPError。
func TestFetchDetail_NonRetryable(t *testing.T) {
	calls := 0
	g := newTestTool(t, "", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	_, err := g.fetchDetail(context.Background(), "o", "n")
	if err == nil {
		t.Fatal("404 应返回错误")
	}
	var gerr *githubHTTPError
	if !errors.As(err, &gerr) {
		t.Fatalf("期望 *githubHTTPError，实际 %T", err)
	}
	if gerr.retryable() {
		t.Fatal("404 不应被判定为可重试")
	}
	if calls != 1 {
		t.Fatalf("非瞬态错误不应重试，期望 1 次调用，实际 %d", calls)
	}
}

// TestMapRepo 验证仓库对象到 schemas.Repo 的字段映射。
func TestMapRepo(t *testing.T) {
	got := mapRepo(repository{
		FullName:        "owner/repo",
		Name:            "repo",
		Owner:           repoOwner{Login: "owner"},
		Description:     "desc",
		Language:        "Go",
		StargazersCount: 42,
		HTMLURL:         "https://github.com/owner/repo",
		Topics:          []string{"a", "b"},
	})
	if got.ID != "owner/repo" || got.Owner != "owner" || got.Name != "repo" ||
		got.Description != "desc" || got.Language != "Go" || got.Stars != 42 ||
		got.URL != "https://github.com/owner/repo" || len(got.Topics) != 2 || got.Topics[0] != "a" {
		t.Fatalf("mapRepo 映射不符: %+v", got)
	}
}

// TestGitHubHTTPError_Retryable 验证错误分类与 Retry-After 解析。
func TestGitHubHTTPError_Retryable(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		retryAfter     string
		wantRetryable  bool
		wantRetryAfter time.Duration
	}{
		{"429 限流带 Retry-After", http.StatusTooManyRequests, "5", true, 5 * time.Second},
		{"429 限流无 Retry-After", http.StatusTooManyRequests, "", true, 0},
		{"500 服务器错误", http.StatusInternalServerError, "", true, 0},
		{"503 服务繁忙", http.StatusServiceUnavailable, "", true, 0},
		{"403 限流（额度耗尽）", http.StatusForbidden, "10", true, 10 * time.Second},
		{"403 非限流", http.StatusForbidden, "", false, 0},
		{"401 鉴权失败", http.StatusUnauthorized, "", false, 0},
		{"404 不存在", http.StatusNotFound, "", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.retryAfter != "" {
				h.Set("Retry-After", tc.retryAfter)
			}
			gerr := newGitHubHTTPError(&http.Response{StatusCode: tc.status, Header: h}, []byte(`{"message":"x"}`))
			if gerr.retryable() != tc.wantRetryable {
				t.Fatalf("retryable() = %v, want %v", gerr.retryable(), tc.wantRetryable)
			}
			if gerr.RetryAfter != tc.wantRetryAfter {
				t.Fatalf("RetryAfter = %v, want %v", gerr.RetryAfter, tc.wantRetryAfter)
			}
		})
	}
}

// TestNewGitHubHTTPError_MessageFallback 验证错误信息的解析与退化。
func TestNewGitHubHTTPError_MessageFallback(t *testing.T) {
	// 标准错误信封：应解析出 message 字段。
	gerr := newGitHubHTTPError(&http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{}}, []byte(`{"message":"boom"}`))
	if gerr.Message != "boom" {
		t.Fatalf("应解析出 message，实际 %q", gerr.Message)
	}
	// 非 JSON 响应体（如网关返回的 HTML）：应退化为原始文本，保证信息不丢失。
	gerr = newGitHubHTTPError(&http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}}, []byte(`<html>bad gateway</html>`))
	if gerr.Message != "<html>bad gateway</html>" {
		t.Fatalf("应退化为原始文本，实际 %q", gerr.Message)
	}
}
