package analysis

// DefinitionRequest 定义了前端发起的“跳转到定义”请求结构
type DefinitionRequest struct {
	RepoID    string `json:"repoId"`    // 仓库 ID
	FilePath  string `json:"filePath"`  // 文件相对路径
	Line      int32  `json:"line"`      // 光标所在行号 (0-based)
	Character int32  `json:"character"` // 光标所在列号 (0-based)
}

// Location 定义了代码中的一个位置范围
type Location struct {
	StartLine   int32 `json:"startLine"`
	StartColumn int32 `json:"startColumn"`
	EndLine     int32 `json:"endLine"`
	EndColumn   int32 `json:"endColumn"`
}

// DefinitionResponse 定义了返回给前端的定义位置信息
type DefinitionResponse struct {
	Kind     string   `json:"kind"`               // 类型: "definition"
	RepoID   string   `json:"repoId"`             // 目标仓库 ID
	FilePath string   `json:"filePath"`           // 目标文件路径
	Range    Location `json:"range"`              // 目标代码范围
	Source   string   `json:"source"`             // ★ 新增: 数据来源 ("scip" | "search")
}