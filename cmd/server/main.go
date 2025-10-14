package main

import (
	"log"
	"net/http"
	"os"

	"code-browser/internal/config"
	"code-browser/internal/core"
	"code-browser/internal/search"
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
	// 1. 加载应用配置
	if err := config.Load("config.json"); err != nil {
		log.Fatalf("无法加载配置文件 config.json: %v", err)
	}
	log.Printf("成功加载 %d 个仓库配置", len(config.GetRepos()))

	// 2. 为每个服务模块创建处理器实例
	coreHandlers := &core.Handlers{}
	searchHandlers := &search.Handlers{}

	// 3. 创建一个路由器并注册处理器方法
	mux := http.NewServeMux()

	// 核心文件浏览服务
	mux.HandleFunc("GET /api/repositories", coreHandlers.ListRepositories)
	mux.HandleFunc("GET /api/repositories/{repoId}/tree", coreHandlers.GetTree)
	mux.HandleFunc("GET /api/repositories/{repoId}/blob", coreHandlers.GetBlob)

	// 搜索服务 (双引擎)
	mux.HandleFunc("GET /api/repositories/{repoId}/search", searchHandlers.SearchContent)
	mux.HandleFunc("GET /api/repositories/{repoId}/search-files", searchHandlers.SearchFiles)

	// 4. 配置静态文件服务
	workDir, _ := os.Getwd()
	webDir := http.Dir(workDir + "/web/")
	fileServer := http.FileServer(webDir)
	mux.Handle("/", fileServer) // 将所有未匹配的路由交给文件服务器
	
	// 5. 启动服务器
	port := ":8088"
	log.Printf("服务器启动，监听端口 %s", port)
	log.Printf("请在浏览器中打开 http://localhost%s/index.html", port)

	handler := corsMiddleware(mux)
	if err := http.ListenAndServe(port, handler); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

