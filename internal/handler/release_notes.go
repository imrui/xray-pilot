package handler

import (
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/pkg/response"
)

// ReleaseNoteItem 单个版本的更新日志
type ReleaseNoteItem struct {
	Version string `json:"version"`
	Content string `json:"content"`
}

// ReleaseNotesHandler 提供内嵌 release-notes 的查询。
// 日志文件在编译期通过 go:embed 打进二进制，内容与运行中的版本天然一致。
type ReleaseNotesHandler struct {
	fsys fs.FS
}

func NewReleaseNotesHandler(fsys fs.FS) *ReleaseNotesHandler {
	return &ReleaseNotesHandler{fsys: fsys}
}

// List 返回全部版本的更新日志，按版本号降序（最新在前）
func (h *ReleaseNotesHandler) List(c *gin.Context) {
	items := make([]ReleaseNoteItem, 0)
	if h.fsys == nil {
		response.Success(c, items)
		return
	}
	names, err := fs.Glob(h.fsys, "v*.md")
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	for _, name := range names {
		data, readErr := fs.ReadFile(h.fsys, name)
		if readErr != nil {
			continue
		}
		items = append(items, ReleaseNoteItem{
			Version: strings.TrimSuffix(name, ".md"),
			Content: string(data),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return compareVersion(items[i].Version, items[j].Version) > 0
	})
	response.Success(c, items)
}

// compareVersion 按数值比较 "v0.4.6" 形式的版本号（字符串排序会把 v0.1.10 排在 v0.1.2 前面）
// a > b 返回正数，相等返回 0
func compareVersion(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			return na - nb
		}
	}
	return 0
}
