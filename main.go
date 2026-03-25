package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/handler"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/scheduler"
	"github.com/imrui/xray-pilot/pkg/logger"
)

//go:embed frontend/dist
var frontendFS embed.FS

func main() {
	// 1. 加载配置
	if err := config.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "配置加载失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 初始化日志
	if err := logger.Init(config.Global.Server.Mode); err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
	log := logger.Log

	// 3. 连接数据库
	if err := repository.Connect(); err != nil {
		log.Fatal("数据库初始化失败", zap.Error(err))
	}
	log.Info("数据库连接成功", zap.String("driver", config.Global.Database.Driver))

	// 4. 启动定时任务（漂移检测 + 健康检测）
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	scheduler.New().Start(schedCtx)

	// 5. 初始化路由
	gin.SetMode(config.Global.Server.Mode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	handler.RegisterRoutes(r)

	// 6. 挂载前端静态文件（embed）
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Warn("前端资源加载失败（开发模式可忽略）", zap.Error(err))
	} else {
		// /assets/* → frontend/dist/assets/
		assetsFS, _ := fs.Sub(distFS, "assets")
		r.StaticFS("/assets", http.FS(assetsFS))

		// 直接读取 index.html 字节，避免 http.FileServer 产生 301 重定向
		indexHTML, _ := fs.ReadFile(distFS, "index.html")
		serveSPA := func(c *gin.Context) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
		}
		r.GET("/", serveSPA)
		// SPA fallback：所有未匹配路由返回 index.html，由前端路由处理
		r.NoRoute(serveSPA)
	}

	// 7. 启动 HTTP 服务器
	addr := fmt.Sprintf(":%d", config.Global.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}
	log.Info("服务启动", zap.String("addr", addr))

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("服务启动失败", zap.Error(err))
		}
	}()

	// 8. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("正在关闭服务...")
	schedCancel() // 先停定时任务
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("服务关闭异常", zap.Error(err))
	}
	log.Info("服务已退出")
}
