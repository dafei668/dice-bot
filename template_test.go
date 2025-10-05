package main

import (
"html/template"
"log"
"os"
)

func main() {
// 测试模板解析
templates, err := template.ParseGlob("web/admin/templates/*.html")
if err != nil {
tf("成功解析的模板:")
for _, tmpl := range templates.Templates() {
tf("- %s", tmpl.Name())
}

// 测试数据
data := map[string]interface{}{
"g": []interface{}{
	6,
	4,
	2,
	"刚刚",
},
 6,
4,
 2,
  "刚刚",
tf("尝试执行dashboard.html模板...")
err = templates.ExecuteTemplate(os.Stdout, "dashboard.html", data)
if err != nil {
tf("执行dashboard.html失败: %v", err)
}

log.Printf("\n\n尝试执行layout模板...")
err = templates.ExecuteTemplate(os.Stdout, "layout", data)
if err != nil {
tf("执行layout失败: %v", err)
}
}
