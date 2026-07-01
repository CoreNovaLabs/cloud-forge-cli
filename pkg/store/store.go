package store

import "context"

// Store 抽象模板仓库：Sync 拉索引，List/Get 检索，GetTemplate 取 IaC 正文。
type Store interface {
	Sync(ctx context.Context) error
	List(filter Filter) ([]App, error)
	Get(appID string) (*App, error)
	GetTemplate(ctx context.Context, appID, cloud string) (body string, err error)
}
