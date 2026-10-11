// Package service — Caddyfile 附加片段(logux-design §7)。
//
// 语义与站点变更一致:"先 Caddy 后 DB"——先用变更后片段集渲染 local 全量产物
// 经管线执行(校验失败即整单拒绝,无任何 DB 变更),成功后才写片段表。
// 片段仅作用于 local 渲染;片段表变更写版本记录(快照由 Site.recordVersion 统一采集)。
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// snippetNameMaxLen 片段名称长度上限(与模型列宽一致)。
const snippetNameMaxLen = 64

// snippetGlobalBlockPattern 顶层全局块:片段文本以顶层匿名块开头(裸 "{")非法。
// 全局指令(admin/log 选项等)归主文件管辖,片段重复全局块必然下发失败,
// 在此提前拦截并给出明确原因;其余语法交由管线内 Caddy 校验兜底。
var snippetGlobalBlockPattern = regexp.MustCompile(`^\s*\{`)

// ErrSnippetNotFound 片段不存在。
var ErrSnippetNotFound = errors.New("snippet not found")

// ErrSnippetNameTaken 片段名称已被占用。
var ErrSnippetNameTaken = errors.New("snippet name taken")

// SnippetRepo snippet service 对数据层的依赖;由 store 实现。
type SnippetRepo interface {
	CreateSnippet(ctx context.Context, snippet *model.Snippet) error
	GetSnippet(ctx context.Context, id uint) (*model.Snippet, error)
	GetSnippetByName(ctx context.Context, name string) (*model.Snippet, error)
	ListAllSnippets(ctx context.Context) ([]model.Snippet, error)
	UpdateSnippet(ctx context.Context, snippet *model.Snippet) error
	DeleteSnippet(ctx context.Context, id uint) error
}

// SnippetLocalDeployer 片段变更下发依赖:用变更后片段集渲染 local 全量产物并经管线执行。
type SnippetLocalDeployer interface {
	DeployLocalWithSnippets(ctx context.Context, snippets []model.Snippet, mode caddy.OpMode) (*caddy.ExecuteResult, error)
}

// SnippetDeps snippet service 依赖。
type SnippetDeps struct {
	Repo     SnippetRepo
	Deployer SnippetLocalDeployer
}

// Snippet 片段业务服务。
type Snippet struct {
	repo     SnippetRepo
	deployer SnippetLocalDeployer
}

// NewSnippet 构造片段服务。
func NewSnippet(d SnippetDeps) *Snippet {
	return &Snippet{repo: d.Repo, deployer: d.Deployer}
}

// SnippetInput 片段新建/编辑输入。
type SnippetInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

// validate 静态校验:名称/内容非空、名称长度、片段禁全局块。
func (in SnippetInput) validate() error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return &InvalidInputError{Reason: "片段名称不能为空"}
	}
	if len(name) > snippetNameMaxLen {
		return &InvalidInputError{Reason: fmt.Sprintf("片段名称过长(最多 %d 字符)", snippetNameMaxLen)}
	}
	if strings.TrimSpace(in.Content) == "" {
		return &InvalidInputError{Reason: "片段内容不能为空"}
	}
	if snippetGlobalBlockPattern.MatchString(in.Content) {
		return &InvalidInputError{Reason: "片段禁止包含全局块(顶层裸花括号块):全局指令归主文件管辖,请使用自包含的站点块或具名片段"}
	}
	return nil
}

// List 返回全部片段(ID 升序 = 渲染追加顺序;纯读无下发)。
func (s *Snippet) List(ctx context.Context) ([]model.Snippet, error) {
	return s.repo.ListAllSnippets(ctx)
}

// Create 新建片段:校验→重名→下发→落库。
func (s *Snippet) Create(ctx context.Context, in SnippetInput) (*model.Snippet, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if _, err := s.repo.GetSnippetByName(ctx, name); err == nil {
		return nil, ErrSnippetNameTaken
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	after, err := s.enabledAfter(ctx, func(existing []model.Snippet) []model.Snippet {
		return append(existing, model.Snippet{Name: name, Content: in.Content, Enabled: in.Enabled})
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.deployer.DeployLocalWithSnippets(ctx, after, caddy.OpDeploy); err != nil {
		return nil, err
	}
	snippet := &model.Snippet{Name: name, Content: in.Content, Enabled: in.Enabled}
	if err := s.repo.CreateSnippet(ctx, snippet); err != nil {
		return nil, err
	}
	return snippet, nil
}

// Update 编辑片段(名称/内容/启用开关):校验→重名→下发→落库。
func (s *Snippet) Update(ctx context.Context, id uint, in SnippetInput) (*model.Snippet, error) {
	snippet, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if owner, err := s.repo.GetSnippetByName(ctx, name); err == nil && owner.ID != id {
		return nil, ErrSnippetNameTaken
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	after, err := s.enabledAfter(ctx, func(existing []model.Snippet) []model.Snippet {
		for i := range existing {
			if existing[i].ID == id {
				existing[i].Name = name
				existing[i].Content = in.Content
				existing[i].Enabled = in.Enabled
			}
		}
		return existing
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.deployer.DeployLocalWithSnippets(ctx, after, caddy.OpDeploy); err != nil {
		return nil, err
	}
	snippet.Name = name
	snippet.Content = in.Content
	snippet.Enabled = in.Enabled
	if err := s.repo.UpdateSnippet(ctx, snippet); err != nil {
		return nil, err
	}
	return snippet, nil
}

// Delete 删除片段:启用片段先重下发移除产物块,再删记录(停用片段直接删)。
func (s *Snippet) Delete(ctx context.Context, id uint) error {
	snippet, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if snippet.Enabled {
		after, err := s.enabledAfter(ctx, func(existing []model.Snippet) []model.Snippet {
			out := make([]model.Snippet, 0, len(existing))
			for _, sn := range existing {
				if sn.ID != id {
					out = append(out, sn)
				}
			}
			return out
		})
		if err != nil {
			return err
		}
		if _, err := s.deployer.DeployLocalWithSnippets(ctx, after, caddy.OpDeploy); err != nil {
			return err
		}
	}
	return s.repo.DeleteSnippet(ctx, id)
}

// get 按 ID 取片段;store.ErrNotFound → 业务错误。
func (s *Snippet) get(ctx context.Context, id uint) (*model.Snippet, error) {
	snippet, err := s.repo.GetSnippet(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrSnippetNotFound
		}
		return nil, err
	}
	return snippet, nil
}

// enabledAfter 构造"变更后启用片段集"(内存计算,不写库):
// mutate 在全量片段列表上应用本次变更,再过滤启用项供渲染下发。
func (s *Snippet) enabledAfter(ctx context.Context, mutate func(existing []model.Snippet) []model.Snippet) ([]model.Snippet, error) {
	all, err := s.repo.ListAllSnippets(ctx)
	if err != nil {
		return nil, err
	}
	after := mutate(all)
	enabled := make([]model.Snippet, 0, len(after))
	for _, sn := range after {
		if sn.Enabled {
			enabled = append(enabled, sn)
		}
	}
	return enabled, nil
}
