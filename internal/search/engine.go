// File: internal/search/engines.go
package search

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// SearchResult 定义了返回给前端的单条搜索结果的结构
type SearchResult struct {
	Path     string `json:"path"`
	LineNum  int    `json:"lineNum"`
	LineText string `json:"lineText"`
}

// Engine 定义了所有搜索引擎都必须实现的接口
type Engine interface {
	SearchContent(repoID, repoPath, query string) ([]SearchResult, error)
	SearchFiles(repoID, repoPath, query string) ([]string, error)
}

// =================================================================================
// Zoekt Engine Implementation
// =================================================================================

type ZoektEngine struct {
	ApiUrl string
}

func (z *ZoektEngine) SearchContent(repoID, repoPath, query string) ([]SearchResult, error) {
	fullQuery := fmt.Sprintf("repo:%s %s", repoID, query)
	resp, err := http.Get(fmt.Sprintf("%s/search?q=%s", z.ApiUrl, url.QueryEscape(fullQuery)))
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Zoekt 服务: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zoekt 服务返回错误, 状态码: %d", resp.StatusCode)
	}

	var zoektResp struct {
		Result struct {
			Files []struct {
				FileName    string `json:"FileName"`
				LineMatches []struct {
					Line       string `json:"Line"`
					LineNumber int    `json:"LineNumber"`
				} `json:"LineMatches"`
			} `json:"Files"`
		} `json:"Result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zoektResp); err != nil {
		return nil, fmt.Errorf("解析 Zoekt JSON 失败: %w", err)
	}

	var results []SearchResult
	for _, fileMatch := range zoektResp.Result.Files {
		for _, lineMatch := range fileMatch.LineMatches {
			results = append(results, SearchResult{
				Path:     fileMatch.FileName,
				LineNum:  lineMatch.LineNumber,
				LineText: strings.TrimSpace(string(lineMatch.Line)),
			})
		}
	}
	return results, nil
}

func (z *ZoektEngine) SearchFiles(repoID, repoPath, query string) ([]string, error) {
	fileQuery := fmt.Sprintf("repo:%s f:%s", repoID, query)
	resp, err := http.Get(fmt.Sprintf("%s/search?q=%s", z.ApiUrl, url.QueryEscape(fileQuery)))
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Zoekt 服务: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zoekt 服务返回错误, 状态码: %d", resp.StatusCode)
	}

	var zoektResp struct {
		Result struct {
			Files []struct {
				FileName string `json:"FileName"`
			} `json:"Files"`
		} `json:"Result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zoektResp); err != nil {
		return nil, fmt.Errorf("解析 Zoekt JSON 失败: %w", err)
	}

	var results []string
	for _, f := range zoektResp.Result.Files {
		results = append(results, f.FileName)
	}
	return results, nil
}

// =================================================================================
// Ripgrep Engine Implementation
// =================================================================================

type RipgrepEngine struct{}

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

func (rg *RipgrepEngine) SearchContent(repoID, repoPath, query string) ([]SearchResult, error) {
	// ★★★ BUG FIX ★★★
	// 恢复使用 cmd.Dir 的标准方式，移除 --cwd 参数
	cmd := exec.Command("rg", "--json", "-i", "-m", "100", query, ".")
	cmd.Dir = repoPath // 正确设置工作目录

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
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				LineNumber uint64 `json:"line_number"`
				Lines      struct {
					Text string `json:"text"`
				} `json:"lines"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &rgResult); err != nil {
			log.Printf("解析 rg JSON 行失败: %v, 行内容: %s", err, line)
			continue
		}
		if rgResult.Type == "match" {
			results = append(results, SearchResult{
				Path:     filepath.ToSlash(rgResult.Data.Path.Text),
				LineNum:  int(rgResult.Data.LineNumber),
				LineText: strings.TrimSpace(rgResult.Data.Lines.Text),
			})
		}
	}

	if err := cmd.Wait(); err != nil {
		// rg 在没有找到匹配项时会以退出码 1 退出，这不是一个真正的错误
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return results, nil // 返回已收集的结果（可能是空的）
		}
		return nil, fmt.Errorf("rg 执行出错: %w", err)
	}
	return results, nil
}

func (rg *RipgrepEngine) SearchFiles(repoID, repoPath, query string) ([]string, error) {
	if query == "" {
		return []string{}, nil
	}

	// ★★★ BUG FIX ★★★
	// 恢复使用 cmd.Dir 并采用旧代码中稳定可靠的 glob 模式
	
	// ripgrep 的 --iglob 参数在某些旧版本中可能不存在，
	// 而 -g "*...*" 结合 -i 标志是更通用的不区分大小写文件名搜索方式。
	// 这里我们转义用户输入以防止注入。
	escapedQuery := escapeGlob(query)
	globPattern := fmt.Sprintf("*%s*", escapedQuery)

	cmd := exec.Command("rg", "--files", "-i", "-g", globPattern)
	cmd.Dir = repoPath // 正确设置工作目录

	output, err := cmd.Output()
	if err != nil {
		// 退出码 1 表示没有匹配，是正常情况
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		// 退出码 2 表示命令本身出错（例如，非法的 glob），需要记录下来
		return nil, fmt.Errorf("rg --files 执行失败: %w", err)
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(files) == 1 && files[0] == "" {
		return []string{}, nil
	}

	// 统一路径分隔符
	for i, f := range files {
		files[i] = filepath.ToSlash(f)
	}

	return files, nil
}

