package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ch3cho/registry-browser/internal/app"
)

// Wails 使用 Go 的 embed 包把前端产物打进可执行文件。
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service, err := app.NewService()
	if err != nil {
		log.Fatalf("failed to initialize service: %v", err)
	}

	desktop := application.New(application.Options{
		Name:        "Registry Browser",
		Description: "浏览 Docker 镜像仓库中的镜像与 tag",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Registry Browser",
		Width:  1280,
		Height: 800,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(250, 250, 248),
		URL:              "/",
	})

	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
