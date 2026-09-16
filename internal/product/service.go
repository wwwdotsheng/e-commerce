package product

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type shopRow struct {
	ID   uuid.UUID `gorm:"column:id"`
	Name string    `gorm:"column:name"`
}

type Service struct {
	db    *gorm.DB
	repo  *Repository
	es    *searchRepo
}

func NewService(db *gorm.DB, repo *Repository, esClient *elasticsearch.Client, logger *zap.Logger) *Service {
	svc := &Service{
		db:   db,
		repo: repo,
	}
	if esClient != nil {
		svc.es = newSearchRepo(esClient, logger)
	}
	return svc
}

func (svc *Service) GetProduct(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	p, err := svc.repo.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errno.ErrProductNotFound
	}
	return p, nil
}

func (svc *Service) CreateProduct(ctx context.Context, param CreateProductParam) error {
	p, err := svc.repo.CreateProduct(ctx, CreateProductData{
		Name:        param.Name,
		Description: param.Description,
		Price:       param.Price,
		Status:      param.Status,
		Stock:       param.Stock,
		Publisher:   param.Publisher,
		ShopID:      param.ShopID,
	})
	if err != nil {
		return err
	}
	if svc.es != nil {
		go svc.es.index(context.Background(), p)
	}
	return nil
}

func (svc *Service) ListProducts(ctx context.Context, param ListProductsParam) ([]*model.Product, int64, error) {
	return svc.repo.ListProducts(ctx, ListProductsData{
		PageNum:  param.PageNum,
		PageSize: param.PageSize,
	})
}

func (svc *Service) ResolveShopNames(ctx context.Context, products []*model.Product) map[string]string {
	ids := make([]uuid.UUID, 0)
	for _, p := range products {
		if p.ShopID != nil {
			ids = append(ids, *p.ShopID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	var rows []shopRow
	if err := svc.db.WithContext(ctx).Model(&model.Shop{}).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.ID.String()] = r.Name
	}
	return m
}

func (svc *Service) SearchProducts(ctx context.Context, param SearchProductsParam) (*SearchProductsResult, error) {
	if svc.es == nil {
		return &SearchProductsResult{Products: []Item{}, Total: 0}, nil
	}
	ids, total, err := svc.es.search(ctx, param.Query, param.MinPrice, param.MaxPrice, param.PageNum, param.PageSize, param.ShopID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &SearchProductsResult{Products: []Item{}, Total: 0}, nil
	}

	// 从 PG 回查最新数据（ES 可能有延迟）
	products, err := svc.repo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	shopNames := svc.ResolveShopNames(ctx, products)

	// 按 ES 返回的顺序排列
	order := make(map[string]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	ordered := make([]Item, len(products))
	for _, p := range products {
		sn := ""
		if p.ShopID != nil {
			sn = shopNames[p.ShopID.String()]
		}
		idx := order[p.ID.String()]
		ordered[idx] = *FormatItem(p, sn)
	}

	return &SearchProductsResult{Products: ordered, Total: total}, nil
}

func (svc *Service) DeleteProduct(ctx context.Context, param DeleteProductParam) error {
	err := svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data: map[string]interface{}{
			"status": model.ProductStatusInactive,
		},
	})
	if err == nil && svc.es != nil {
		go svc.es.remove(context.Background(), param.ProductID.String())
	}
	return err
}

func (svc *Service) UpdateProductProperty(ctx context.Context, param UpdateProductPropertyParam) error {
	updateData := map[string]interface{}{}
	if param.Name != nil {
		updateData["name"] = *param.Name
	}
	if param.Description != nil {
		updateData["description"] = *param.Description
	}
	if param.Price != nil {
		updateData["price"] = *param.Price
	}

	err := svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data:      updateData,
	})
	if err == nil && svc.es != nil {
		// 异步同步到 ES
		p, _ := svc.repo.GetProduct(context.Background(), param.ProductID)
		if p != nil {
			go svc.es.index(context.Background(), p)
		}
	}
	return err
}

func (svc *Service) UpdateProductStatus(ctx context.Context, param UpdateProductStatusParam) error {
	if !param.Status.IsValid() {
		return errno.ErrProductStatusInvalid
	}
	return svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data: map[string]interface{}{
			"status": param.Status,
		},
	})
}

func (svc *Service) UpdateProductStock(ctx context.Context, param UpdateProductStockParam) error {
	return svc.repo.UpdateStock(ctx, UpdateStockData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Quantity:  param.Quantity,
		Reason:    param.Reason,
	})
}

func (svc *Service) RebuildIndex(ctx context.Context, products []*model.Product) {
	if svc.es != nil {
		_ = svc.es.rebuildIndex(ctx, products)
	}
}
