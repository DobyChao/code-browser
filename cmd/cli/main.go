package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"code-browser/internal/repo"
)

func main() {
	// --- Define Flags ---
	// Command flag determines the action
	command := flag.String("command", "", "操作命令: 'add', 'delete' 或 'index' (必填)")
	// Common flags
	dataDir := flag.String("data-dir", "./.data", "应用程序的全局数据目录")
	// A single ID flag used by both 'add' and 'delete' commands
	repoID := flag.Uint("id", 0, "仓库的唯一数字 ID (必填, 用于 'add', 'delete' 或 'index' 命令)")
	// Flags for 'add' command
	repoName := flag.String("name", "", "'add' 命令: 仓库的显示名称 (必填)")
	repoPath := flag.String("path", "", "'add' 命令: 仓库源代码的绝对路径 (必填)")
	// Flags for 'delete' command
	// --- Parse Flags ---
	flag.Parse()
	// --- Initialize Repository Provider ---
	log.Printf("使用数据目录: %s", *dataDir)
	repoProvider, err := repo.NewProvider(*dataDir)
	if err != nil {
		log.Fatalf("错误: 无法初始化仓库服务: %v", err)
	}
	defer func() {
		if err := repoProvider.Close(); err != nil {
			log.Printf("关闭数据库连接时出错: %v", err)
		}
	}()

	// --- Execute Command ---
	switch *command {
	case "add":
		if *repoID == 0 || *repoName == "" || *repoPath == "" {
			fmt.Fprintln(os.Stderr, "错误: 'add' 命令需要 -id, -name, 和 -path 参数。")
			os.Exit(1)
		}
		err = repoProvider.AddRepository(uint32(*repoID), *repoName, *repoPath)
		if err != nil {
			log.Fatalf("错误: 添加仓库失败: %v", err)
		}
		fmt.Printf("成功添加仓库: ID=%d, Name=%s\n", *repoID, *repoName)

	case "delete":
		// Validate required flags for delete
		if *repoID == 0 {
			// Note: The original code had a fallback for deleteID = 0 to addID.
			// With a single repoID, this fallback is no longer needed.
			// The check simply ensures that the ID is provided.
			fmt.Fprintln(os.Stderr, "错误: 'delete' 命令需要 -id 参数.")
			os.Exit(1)
		}
		// Call DeleteRepository
		err = repoProvider.DeleteRepository(uint32(*repoID))
		if err != nil {
			log.Fatalf("错误: 删除仓库失败: %v", err)
		}
		fmt.Printf("成功删除仓库: ID=%d\n", *repoID)

	case "index":
		if *repoID == 0 {
			fmt.Fprintln(os.Stderr, "错误: 'index' 命令需要 -id 参数。")
			os.Exit(1)
		}

		err = repoProvider.IndexRepositoryZoekt(uint32(*repoID))
		if err != nil {
			log.Fatalf("错误: 索引仓库失败: %v", err)
		}
		fmt.Printf("成功触发仓库 %d 的 Zoekt 索引生成。\n", *repoID)

	default:
		fmt.Fprintf(os.Stderr, "错误: 无效或未指定命令 '%s'\n\n", *command)
		os.Exit(1)
	}
}
