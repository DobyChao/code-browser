package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"code-browser/internal/analysis"
	"code-browser/internal/core"
	"code-browser/internal/repo"
	"code-browser/internal/search"

	"github.com/patrickmn/go-cache"
)

// corsMiddleware 为所有响应添加 CORS 头
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	// 1. 定义命令行参数
	dataDir := flag.String("data-dir", "./.data", "应用程序的全局数据目录 (包含数据库和仓库数据)")
	adminToken := flag.String("admin-token", "", "管理 API 的鉴权 Token (如果为空则不开启鉴权)")
	flag.Parse()

	log.Printf("使用数据目录: %s", *dataDir)

	// 2. 创建仓库管理服务实例
	repoProvider, err := repo.NewProvider(*dataDir)
	if err != nil {
		log.Fatalf("错误: 无法初始化仓库服务: %v", err)
	}
	defer func() {
		if err := repoProvider.Close(); err != nil {
			log.Printf("关闭数据库连接时出错: %v", err)
		}
	}()

	log.Printf("成功加载并初始化 %d 个仓库", repoProvider.Count())

	appCache := cache.New(5*time.Minute, 10*time.Minute)

	coreService := core.NewService(repoProvider, appCache)

	zoektEngine := &search.ZoektEngine{ApiUrl: "http://localhost:6070"}
	ripgrepEngine := &search.RipgrepEngine{}

	// 3. 创建并配置搜索服务
	searchHandlers := &search.Handlers{
		RepoProvider: repoProvider,
		Engines: map[string]search.Engine{
			"zoekt":   zoektEngine,
			"ripgrep": ripgrepEngine,
		},
		Cache: appCache,
	}

	// 4. 创建核心服务
	coreHandlers := &core.Handlers{
		RepoProvider: repoProvider,
		Service:      coreService,
	}

	analysisService := analysis.NewService(repoProvider, zoektEngine, coreService)
	analysisHandlers := &analysis.Handlers{Service: analysisService}

	// 5.1 创建仓库管理 Handler
	repoHandlers := &repo.Handlers{
		Provider:   repoProvider,
		AdminToken: *adminToken,
	}

	// 5. 创建路由器并集中注册所有服务的路由 (恢复简洁方式)
	mux := http.NewServeMux()

	// 静态文件服务
	mux.Handle("GET /", http.FileServer(http.Dir("web"))) // Serve static files from web directory

	// 仓库管理 API (受 AuthMiddleware 保护)
	mux.HandleFunc("GET /api/admin/repositories", repoHandlers.AuthMiddleware(repoHandlers.HandleListAdmin))
	mux.HandleFunc("POST /api/repositories", repoHandlers.AuthMiddleware(repoHandlers.HandleAdd))
	mux.HandleFunc("DELETE /api/repositories/{id}", repoHandlers.AuthMiddleware(repoHandlers.HandleDelete))
	mux.HandleFunc("POST /api/repositories/{id}/index", repoHandlers.AuthMiddleware(repoHandlers.HandleIndex))
	mux.HandleFunc("POST /api/repositories/{id}/scip", repoHandlers.AuthMiddleware(repoHandlers.HandleRegisterScip))
	mux.HandleFunc("POST /api/repositories/{id}/zoekt-file", repoHandlers.AuthMiddleware(repoHandlers.HandleRegisterZoekt))

	// 核心文件浏览服务 (处理器内部解析 {id})
	mux.HandleFunc("GET /api/repositories", coreHandlers.ListRepositories)
	mux.HandleFunc("GET /api/repositories/{id}/tree", coreHandlers.GetTree)
	mux.HandleFunc("GET /api/repositories/{id}/blob", coreHandlers.GetBlob)

	// 搜索服务 (处理器内部解析 {id})
	mux.HandleFunc("GET /api/repositories/{id}/search", searchHandlers.SearchContent)
	mux.HandleFunc("GET /api/repositories/{id}/search-files", searchHandlers.SearchFiles)

	mux.HandleFunc("POST /api/intelligence/definitions", analysisHandlers.GetDefinitionHandler)
	mux.HandleFunc("POST /api/intelligence/references", analysisHandlers.GetReferencesHandler)

	// 6. 配置并启动服务器
	server := &http.Server{
		Addr:         ":8088",
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("服务器启动，监听端口 :8088")
	log.Println("请在浏览器中打开 http://localhost:8088/")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
