package store

import "time"

// Catalog 是 index/apps.json 的顶层结构。
type Catalog struct {
	CatalogVersion string    `json:"catalog_version"`
	GeneratedAt    string    `json:"generated_at"`
	BaseURL        string    `json:"base_url"`
	Store          StoreMeta `json:"store"`
	Apps           []App     `json:"apps"`
}

type StoreMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// App 对应 catalog 中单个应用条目。
type App struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Desc          string                     `json:"desc"`
	Icon          string                     `json:"icon,omitempty"`
	Category      string                     `json:"category"`
	Tags          []string                   `json:"tags,omitempty"`
	Stars         int                        `json:"stars,omitempty"`
	Clouds        []string                   `json:"clouds"`
	Version       string                     `json:"version"`
	MinCLIVersion string                     `json:"min_cli_version,omitempty"`
	Price         string                     `json:"price,omitempty"`
	Images        map[string]string          `json:"images"`
	Templates     map[string]TemplateRef     `json:"templates"`
	Params        map[string]ParamDefinition `json:"params,omitempty"`
}

type TemplateRef struct {
	Path     string `json:"path"`
	URL      string `json:"url,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

type ParamDefinition struct {
	Type     string      `json:"type,omitempty"`
	Secret   bool        `json:"secret,omitempty"`
	Required bool        `json:"required,omitempty"`
	Default  interface{} `json:"default,omitempty"`
	Options  []string    `json:"options,omitempty"`
	AWS      *CloudParam `json:"aws,omitempty"`
	Aliyun   *CloudParam `json:"aliyun,omitempty"`
}

type CloudParam struct {
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// Filter 用于 search / list 内存检索。
type Filter struct {
	Query    string
	Category string
	Cloud    string
	Tags     []string
}

// Config 控制 Store 客户端行为。
type Config struct {
	// IndexURL apps.json 地址，支持 https:// 或 file://
	IndexURL string
	// CacheDir 本地缓存目录，默认 ~/.cloud-forge/cache
	CacheDir string
	// CacheTTL 索引缓存有效期
	CacheTTL time.Duration
	// HTTPTimeout controls remote index/template fetch timeout.
	HTTPTimeout time.Duration
}
