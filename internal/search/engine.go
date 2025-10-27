package search

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url" // 引入 net/url
	"os/exec"
	"path/filepath"
	"strings"

	"code-browser/internal/repo"
)

// SearchFragment 定义了行内的一个匹配片段
type SearchFragment struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

// SearchResult 定义了返回给前端的单条搜索结果的结构 (已更新)
type SearchResult struct {
	Path      string           `json:"path"`
	LineNum   int              `json:"lineNum"`
	LineText  string           `json:"lineText"`  // 完整的、base64 解码后的行文本
	Fragments []SearchFragment `json:"fragments"` // 行内的匹配片段列表
}

// Engine 定义了所有搜索引擎都必须实现的接口 (保持不变)
type Engine interface {
	SearchContent(repo repo.Repository, query string) ([]SearchResult, error)
	SearchFiles(repo repo.Repository, query string) ([]string, error)
}

// =================================================================================
// Zoekt Engine Implementation
// =================================================================================

type ZoektEngine struct {
	ApiUrl string // 应该是 http://localhost:6070
}

// --- Zoekt JSON API 响应结构 (根据您的示例定义) ---
// (这些结构与 GET /search?format=json 的响应结构一致)
type ZoektApiSearchResult struct {
	Result *ZoektResultInput `json:"Result,omitempty"`
}

type ZoektResultInput struct {
	FileMatches []*ZoektFileMatch `json:"Files,omitempty"`
}

type ZoektFileMatch struct {
	FileName string       `json:"FileName"`
	Repo     string       `json:"Repository"`
	Matches  []ZoektMatch `json:"LineMatches,omitempty"`
}

type ZoektMatch struct {
	Line          string          `json:"Line"` // base64 encoded
	LineNumber    int             `json:"LineNumber"`
	LineFragments []ZoektFragment `json:"LineFragments"` // 捕获片段信息
}

type ZoektFragment struct {
	LineOffset  int `json:"LineOffset"` // 列偏移量
	MatchLength int `json:"MatchLength"`
}

// --- Zoekt JSON API 请求结构 (已更新) ---

// ZoektSearchOptions 定义了可以传递给 Zoekt 的搜索选项
type ZoektSearchOptions struct {
	TotalMaxMatchCount int `json:"TotalMaxMatchCount,omitempty"`
}

type zoektSearchRequest struct {
	Q       string              `json:"Q"`
	RepoIDs []uint32            `json:"RepoIDs,omitempty"`
	Opts    *ZoektSearchOptions `json:"Opts,omitempty"` // 添加 Opts 字段
}

// --- ZoektEngine 方法实现 (已更新) ---

func (z *ZoektEngine) doZoektRequest(payload any) (*ZoektApiSearchResult, error) {
	// 1. 构建 URL
	searchURL, err := url.Parse(z.ApiUrl)
	if err != nil {
		return nil, fmt.Errorf("无效的 Zoekt API URL: %w", err)
	}
	searchURL = searchURL.JoinPath("api", "search") // 拼接 /api/search

	// 2. 序列化请求体
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 Zoekt 请求体失败: %w", err)
	}

	// 3. 添加调试日志
	log.Printf("DEBUG: 正在向 Zoekt 发送 POST 请求...")
	log.Printf("DEBUG: URL: %s", searchURL.String())
	log.Printf("DEBUG: Body: %s", string(body))

	// 4. 发送 POST 请求
	req, err := http.NewRequest("POST", searchURL.String(), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("创建 Zoekt POST 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("ERROR: 请求 Zoekt API 失败: %v", err)
		return nil, fmt.Errorf("无法连接到 Zoekt 服务 (%s): %w", searchURL.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("DEBUG: Zoekt 返回的错误 Body: %s", string(bodyBytes))
		return nil, fmt.Errorf("Zoekt 服务返回错误, 状态码: %d", resp.StatusCode)
	}

	// 5. 解析为响应结构
	var zoektResp ZoektApiSearchResult
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 Zoekt 响应体失败: %w", err)
	}
	if err := json.Unmarshal(bodyBytes, &zoektResp); err != nil {
		log.Printf("DEBUG: 无法解析的 Zoekt JSON 响应: %s", string(bodyBytes))
		return nil, fmt.Errorf("解析 Zoekt JSON 失败: %w", err)
	}

	if zoektResp.Result == nil {
		// 注意: Zoekt 在没有结果时，可能会返回一个空的 `{"Result": {}}`，这不一定是错误
		// 但如果连 Result 字段都没有，那一定是格式错了
		if !strings.Contains(string(bodyBytes), `"Result"`) {
			log.Printf("DEBUG: Zoekt 响应体中缺少 'Result' 字段: %s", string(bodyBytes))
			return nil, fmt.Errorf("Zoekt 响应格式无效 (缺少 'Result')")
		}
		// 否则，只是没有匹配，返回一个空结果
		return &ZoektApiSearchResult{Result: &ZoektResultInput{}}, nil
	}

	return &zoektResp, nil
}

func (z *ZoektEngine) SearchContent(repo repo.Repository, query string) ([]SearchResult, error) {
	// ★★★ 核心改动: 添加 Opts 字段 ★★★
	payload := zoektSearchRequest{
		Q:       query,
		RepoIDs: []uint32{repo.RepoID},
		Opts:    &ZoektSearchOptions{TotalMaxMatchCount: 1000},
	}

	zoektResp, err := z.doZoektRequest(payload)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	// 检查 zoektResp.Result 是否为 nil (在 doZoektRequest 中已保证不为 nil)
	// 但 FileMatches 可能为 nil
	if zoektResp.Result.FileMatches == nil {
		return results, nil // 没有匹配，返回空列表
	}

	for _, fileMatch := range zoektResp.Result.FileMatches {
		for _, match := range fileMatch.Matches {
			// ★★★ 核心改动: 解析 Line 和 LineFragments ★★★

			// 1. 解码 base64 行文本
			lineTextBytes, err := base64.StdEncoding.DecodeString(match.Line)
			if err != nil {
				log.Printf("WARN: 解码 Zoekt base64 内容失败 (%s): %v", match.Line, err)
				continue
			}
			lineText := string(lineTextBytes)

			// 2. 转换 Fragments 结构
			var apiFragments []SearchFragment
			for _, frag := range match.LineFragments {
				apiFragments = append(apiFragments, SearchFragment{
					Offset: frag.LineOffset,
					Length: frag.MatchLength,
				})
			}

			// 3. 填充新的 SearchResult 结构
			results = append(results, SearchResult{
				Path:      fileMatch.FileName,
				LineNum:   match.LineNumber,
				LineText:  lineText,
				Fragments: apiFragments,
			})
		}
	}
	return results, nil
}

func (z *ZoektEngine) SearchFiles(repo repo.Repository, query string) ([]string, error) {
	fileQuery := fmt.Sprintf("f:%s", query)
	// ★★★ 核心改动: 添加 Opts 字段 ★★★
	payload := zoektSearchRequest{
		Q:       fileQuery,
		RepoIDs: []uint32{repo.RepoID},
		Opts:    &ZoektSearchOptions{TotalMaxMatchCount: 1000},
	}

	zoektResp, err := z.doZoektRequest(payload)
	if err != nil {
		return nil, err
	}

	var results []string
	if zoektResp.Result.FileMatches == nil {
		return results, nil // 没有匹配，返回空列表
	}

	for _, f := range zoektResp.Result.FileMatches {
		results = append(results, f.FileName)
	}
	return results, nil
}

// =================================================================================
// Ripgrep Engine Implementation
// =================================================================================

type RipgrepEngine struct{}

func (rg *RipgrepEngine) SearchContent(repo repo.Repository, query string) ([]SearchResult, error) {
	cmd := exec.Command("rg", "--json", "-i", "-m", "100", query, ".")
	cmd.Dir = repo.SourcePath // 使用正确的字段名

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 rg 管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 rg 失败: %w", err)
	}

	var results []SearchResult
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		var rgResult struct {
			Type string `json:"type"`
			Data struct {
				Path       struct{ Text string `json:"text"` } `json:"path"`
				LineNumber uint64                              `json:"line_number"`
				Lines      struct{ Text string `json:"text"` } `json:"lines"`
				// ★★★ 核心改动: 捕获 Submatches ★★★
				Submatches []struct {
					Start int `json:"start"`
					End   int `json:"end"`
				} `json:"submatches"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &rgResult); err != nil {
			log.Printf("解析 rg JSON 行失败: %v, 行内容: %s", err, line)
			continue
		}
		if rgResult.Type == "match" {
			// ★★★ 核心改动: 转换 Submatches ★★★
			var apiFragments []SearchFragment
			lineText := strings.TrimSpace(rgResult.Data.Lines.Text)
			
			for _, submatch := range rgResult.Data.Submatches {
				// rg 的 offset 是基于原始行（包含换行符）的，
				// 而我们 TrimSpace 了。为简单起见，我们假设匹配不在前导/后导空格中。
				// 更健壮的方法是计算前导空格的长度。
				// 但对于大多数代码文件，TrimSpace 影响不大。
				apiFragments = append(apiFragments, SearchFragment{
					Offset: submatch.Start,
					Length: submatch.End - submatch.Start,
				})
			}

			results = append(results, SearchResult{
				Path:      filepath.ToSlash(rgResult.Data.Path.Text),
				LineNum:   int(rgResult.Data.LineNumber),
				LineText:  lineText,
				Fragments: apiFragments, // 填充 Fragments
			})
		}
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return results, nil
		}
		return nil, fmt.Errorf("rg 执行出错: %w", err)
	}
	return results, nil
}

func (rg *RipgrepEngine) SearchFiles(repo repo.Repository, query string) ([]string, error) {
	if query == "" {
		return []string{}, nil
	}
	escapedQuery := escapeGlob(query)
	globPattern := fmt.Sprintf("*%s*", escapedQuery)

	cmd := exec.Command("rg", "--files", "--iglob", globPattern)
	cmd.Dir = repo.SourcePath // 使用正确的字段名

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("rg --files 执行失败: %w", err)
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	results := make([]string, len(files))
	for i, f := range files {
		results[i] = filepath.ToSlash(f)
	}
	return results, nil
}

// escapeGlob 转义 glob 模式中的特殊字符 (*, ?, [)
func escapeGlob(pattern string) string {
	var sb strings.Builder
	for _, r := range pattern {
		if r == '*' || r == '?' || r == '[' {
			sb.WriteRune('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
