package analysis

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"unicode"

	"code-browser/internal/core" // ★ 引入 core 包
	"code-browser/internal/repo"
	"code-browser/internal/search"

	"github.com/patrickmn/go-cache" // ★ 引入 cache 包
	"github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

type Service struct {
	RepoProvider *repo.Provider
	SearchEngine search.Engine
	CoreService  *core.Service // ★ 注入 CoreService
	ScipCache    *cache.Cache  // ★ SCIP 索引缓存
}

// NewService 创建一个新的分析服务
func NewService(repoProvider *repo.Provider, searchEngine search.Engine, coreService *core.Service) *Service {
	// SCIP 索引文件通常较大，但解析结构体相对较小，且访问频率高。
	// 设置较长的过期时间，例如 1 小时。
	scipCache := cache.New(cache.NoExpiration, cache.NoExpiration)
	return &Service{
		RepoProvider: repoProvider,
		SearchEngine: searchEngine,
		CoreService:  coreService,
		ScipCache:    scipCache,
	}
}

// GetDefinition 查找给定位置符号的定义
func (s *Service) GetDefinition(req DefinitionRequest) ([]DefinitionResponse, error) {
	repoID := s.RepoProvider.GetRepoIDByString(req.RepoID)
	if repoID == 0 {
		return nil, fmt.Errorf("仓库 '%s' 未找到", req.RepoID)
	}

	repoInfo, ok := s.RepoProvider.GetRepo(repoID)
	if !ok {
		return nil, fmt.Errorf("仓库 ID '%d' 未找到", repoID)
	}

	scipPath := filepath.Join(repoInfo.DataPath, "scip", "index.scip")

	log.Printf("DEBUG: 检查 SCIP 参数 (%s) (%d) (%d) (%s)", scipPath, req.Line, req.Character, req.RepoID)
	// ★ 优化: 优先检查缓存，如果缓存没有再检查文件状态
	if _, found := s.ScipCache.Get(scipPath); found {
		return s.getDefinitionFromSCIP(scipPath, req.FilePath, req.Line, req.Character, req.RepoID)
	}

	if _, err := os.Stat(scipPath); err == nil {
		log.Printf("DEBUG: 检查 SCIP 参数 (%s) (%d) (%d) (%s)", scipPath, req.Line, req.Character, req.RepoID)
		defs, err := s.getDefinitionFromSCIP(scipPath, req.FilePath, req.Line, req.Character, req.RepoID)
		if err == nil && len(defs) > 0 {
			log.Printf("DEBUG: SCIP 命中定义 (%s)", req.FilePath)
			return defs, nil
		}
	}

	return s.getDefinitionFromSearch(repoInfo, req.FilePath, req.Line, req.Character)
}

// getDefinitionFromSearch 使用搜索引擎尝试查找定义
func (s *Service) getDefinitionFromSearch(repoInfo repo.Repository, filePath string, line, char int32) ([]DefinitionResponse, error) {
	// ★ 优化: 使用 CoreService 获取文件内容，利用其缓存 ★
	content, _, err := s.CoreService.GetFileContent(repoInfo.RepoID, filePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取源文件以提取符号: %w", err)
	}

	// 使用 bytes.Reader 和 bufio 读取内存中的内容
	reader := bytes.NewReader(content)
	scanner := bufio.NewScanner(reader)

	currentLine := int32(0)
	var lineText string
	for scanner.Scan() {
		if currentLine == line {
			lineText = scanner.Text()
			break
		}
		currentLine++
	}

	symbol := extractWordAtPosition(lineText, int(char))
	if symbol == "" {
		return nil, fmt.Errorf("光标处未找到有效符号")
	}
	log.Printf("DEBUG: Fallback 搜索符号: %s", symbol)

	var query string
	if _, ok := s.SearchEngine.(*search.ZoektEngine); ok {
		query = fmt.Sprintf("sym:%s", symbol)
	} else {
		query = fmt.Sprintf("\\b%s\\b", symbol)
	}

	searchResults, err := s.SearchEngine.SearchContent(repoInfo, query)

	if err != nil || len(searchResults) == 0 {
		if _, ok := s.SearchEngine.(*search.ZoektEngine); ok {
			log.Printf("DEBUG: 符号搜索无结果，尝试纯文本全字匹配")
			query = fmt.Sprintf("\\b%s\\b", symbol)
			searchResults, err = s.SearchEngine.SearchContent(repoInfo, query)
		}
	}

	if err != nil {
		return nil, err
	}

	var definitions []DefinitionResponse
	repoIDStr := strconv.FormatUint(uint64(repoInfo.RepoID), 10)
	for _, res := range searchResults {
		def := DefinitionResponse{
			Kind:     "search-result",
			RepoID:   repoIDStr,
			FilePath: res.Path,
			Range: Location{
				StartLine:   int32(res.LineNum),
				StartColumn: 0,
				EndLine:     int32(res.LineNum),
				EndColumn:   0,
			},
			Source: "search",
		}
		definitions = append(definitions, def)
		if len(definitions) >= 10 {
			break
		}
	}

	return definitions, nil
}

// getDefinitionFromSCIP 封装原有的 SCIP 逻辑
func (s *Service) getDefinitionFromSCIP(scipPath, filePath string, line, char int32, repoIDStr string) ([]DefinitionResponse, error) {
	// ★ 优化: 从缓存读取 SCIP 索引 ★
	var index *scip.Index
	if data, found := s.ScipCache.Get(scipPath); found {
		index = data.(*scip.Index)
	} else {
		log.Printf("DEBUG: 加载 SCIP 索引到缓存: %s", scipPath)
		var err error
		index, err = readSCIPIndex(scipPath)
		if err != nil {
			return nil, err
		}
		// 缓存解析后的对象
		s.ScipCache.Set(scipPath, index, cache.DefaultExpiration)
	}

	log.Printf("DEBUG: SCIP 搜索符号: %s (%d) (%d)", filePath, line, char)

	var targetDoc *scip.Document
	for _, doc := range index.Documents {
		if doc.RelativePath == filePath {
			targetDoc = doc
			break
		}
	}
	if targetDoc == nil {
		return nil, fmt.Errorf("doc not found")
	}

	symbol := findSymbolAtPosition(targetDoc, line, char)
	if symbol == "" {
		return nil, fmt.Errorf("symbol not found")
	}

	var definitions []DefinitionResponse
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			if occ.Symbol == symbol && (occ.SymbolRoles&int32(scip.SymbolRole_Definition) != 0) {
				def := DefinitionResponse{
					Kind:     "definition",
					RepoID:   repoIDStr,
					FilePath: doc.RelativePath,
					Range: Location{
						StartLine:   occ.Range[0] + 1,
						StartColumn: occ.Range[1],
						EndLine:     occ.Range[0] + 1,
						EndColumn:   occ.Range[1],
					},
					Source: "scip",
				}
				if len(occ.Range) == 4 {
					def.Range.EndLine = occ.Range[2] + 1
					def.Range.EndColumn = occ.Range[3]
				} else if len(occ.Range) == 3 {
					def.Range.EndLine = occ.Range[0] + 1
					def.Range.EndColumn = occ.Range[2]
				}
				definitions = append(definitions, def)
			}
		}
	}
	return definitions, nil
}

func readSCIPIndex(path string) (*scip.Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var index scip.Index
	if err := proto.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

func (s *Service) getDefinitionFromSCIPForTest(index *scip.Index, filePath string, line, char int32, repoIDStr string) ([]DefinitionResponse, error) {
	var targetDoc *scip.Document
	for _, doc := range index.Documents {
		if doc.RelativePath == filePath {
			targetDoc = doc
			break
		}
	}
	if targetDoc == nil {
		return nil, fmt.Errorf("doc not found")
	}

	symbol := findSymbolAtPosition(targetDoc, line, char)
	if symbol == "" {
		return nil, fmt.Errorf("symbol not found")
	}

	var definitions []DefinitionResponse
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			if occ.Symbol == symbol && (occ.SymbolRoles&int32(scip.SymbolRole_Definition) != 0) {
				def := DefinitionResponse{
					Kind:     "definition",
					RepoID:   repoIDStr,
					FilePath: doc.RelativePath,
					Range: Location{
						StartLine:   occ.Range[0] + 1,
						StartColumn: occ.Range[1],
						EndLine:     occ.Range[0] + 1,
						EndColumn:   occ.Range[1],
					},
					Source: "scip",
				}
				if len(occ.Range) == 4 {
					def.Range.EndLine = occ.Range[2] + 1
					def.Range.EndColumn = occ.Range[3]
				} else if len(occ.Range) == 3 {
					def.Range.EndLine = occ.Range[0] + 1
					def.Range.EndColumn = occ.Range[2]
				}
				definitions = append(definitions, def)
			}
		}
	}
	return definitions, nil
}

func findSymbolAtPosition(doc *scip.Document, line, char int32) string {
	for _, occ := range doc.Occurrences {
		startLine := occ.Range[0]
		startChar := occ.Range[1]
		endLine := startLine
		endChar := int32(0)
		if len(occ.Range) == 3 {
			endChar = occ.Range[2]
		} else {
			endLine = occ.Range[2]
			endChar = occ.Range[3]
		}
		if line < startLine || line > endLine {
			continue
		}
		if startLine == endLine {
			if char >= startChar && char < endChar {
				return occ.Symbol
			}
		} else {
			if line == startLine {
				if char >= startChar {
					return occ.Symbol
				}
			} else if line == endLine {
				if char < endChar {
					return occ.Symbol
				}
			} else {
				return occ.Symbol
			}
		}
	}
	return ""
}

func extractWordAtPosition(lineText string, column int) string {
	runes := []rune(lineText)
	if column < 0 || column >= len(runes) {
		return ""
	}
	if !isWordChar(runes[column]) {
		if column > 0 && isWordChar(runes[column-1]) {
			column--
		} else {
			return ""
		}
	}
	start := column
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}
	end := column
	for end < len(runes) && isWordChar(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
