package main

import (
	"html/template"
	"log"
	"os"
)

func init() {
	// 测试模板解析
	templates, err := template.ParseGlob("web/admin/templates/*.html")
	if err != nil {
		log.Fatal("模板解析失败:", err)
	}

	log.Printf("成功解析的模板:")
	for _, tmpl := range templates.Templates() {
		log.Printf("- %s", tmpl.Name())
	}

	// 测试数据
	data := map[string]interface{}{
		"Title": "仪表板",
		"Stats": map[string]interface{}{
			"total_users":    6,
			"active_users":   4,
			"total_games":    2,
			"total_recharge": 0.0,
			"update_time":    "刚刚",
		},
	}

	// 尝试执行dashboard.html模板
	log.Printf("尝试执行dashboard.html模板...")
	err = templates.ExecuteTemplate(os.Stdout, "dashboard.html", data)
	if err != nil {
		log.Printf("执行dashboard.html失败: %v", err)
	}

	log.Printf("\n\n尝试执行layout模板...")
	err = templates.ExecuteTemplate(os.Stdout, "layout", data)
	if err != nil {
		log.Printf("执行layout失败: %v", err)
	}
}
