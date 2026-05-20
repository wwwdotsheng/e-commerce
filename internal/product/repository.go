package product

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/errno"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	*database.BaseRepo
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{BaseRepo: database.NewBaseRepo(db)}
}

type CreateProductData struct {
	Name        string
	Description string
	Price       float64
	Status      *model.ProductStatus
	Stock       int
	Publisher   uuid.UUID
}

func (repo *Repository) CreateProduct(ctx context.Context, data CreateProductData) error {
	pStatus := model.ProductStatusInactive
	if data.Status != nil && data.Status.IsValid() {
		pStatus = *data.Status
	}

	p := &model.Product{
		Publisher:   data.Publisher,
		Name:        data.Name,
		Description: data.Description,
		Price:       data.Price,
		Stock:       data.Stock,
		Status:      pStatus,
		Version:     1,
	}
	return repo.GetDB(ctx).Create(p).Error
}

func (repo *Repository) GetProductByID(ctx context.Context, id uuid.UUID, lockType database.LockType) (*model.Product, error) {
	var p model.Product
	db := repo.GetDB(ctx)

	switch lockType {
	case database.LockUpdate:
		db = db.Clauses(clause.Locking{Strength: string(database.LockUpdate)})
	case database.LockShare:
		db = db.Clauses(clause.Locking{Strength: string(database.LockShare)})
	}
	err := db.First(&p, "id = ?", id).Error
	return &p, err
}

func (repo *Repository) GetProduct(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	p := &model.Product{}
	db := repo.GetDB(ctx)

	err := db.
		Where("id = ?", id).
		First(p).
		Error
	return p, err
}

type UpdateStockData struct {
	ProductID uuid.UUID
	Publisher uuid.UUID
	Quantity  int
	Reason    model.StockChangeReason
}

// createStockChangeLog 记录库存变动
func (repo *Repository) createStockChangeLog(ctx context.Context, productID uuid.UUID, quantity, before int, reason model.StockChangeReason) error {
	return repo.GetDB(ctx).Create(&model.StockChangeLog{
		ProductID: productID,
		Quantity:  quantity,
		Before:    before,
		After:     before + quantity,
		Reason:    reason,
	}).Error
}

// UpdateStock 更新商品库存（校验 publisher）
func (repo *Repository) UpdateStock(ctx context.Context, data UpdateStockData) error {
	db := repo.GetDB(ctx).Model(&model.Product{}).Where("id = ? and publisher = ?", data.ProductID, data.Publisher)
	if data.Quantity < 0 {
		db = db.Where("stock >= ?", -data.Quantity)
	}

	var result struct {
		ID    uuid.UUID
		Stock int
	}
	err := db.
		Clauses(clause.Returning{Columns: []clause.Column{
			{Name: "id"},
			{Name: "stock"},
		}}).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock + ?", data.Quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		}).
		Scan(&result).Error
	if err != nil {
		return err
	}

	if result.ID == uuid.Nil {
		return errors.New("update failed: insufficient stock or product not found")
	}

	return repo.createStockChangeLog(ctx, data.ProductID, data.Quantity, result.Stock-data.Quantity, data.Reason)
}

// DeductStock 下单扣减库存（无 publisher 校验，事务内使用）
func (repo *Repository) DeductStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	var result struct {
		ID    uuid.UUID
		Stock int
	}
	err := repo.GetDB(ctx).Model(&model.Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		Clauses(clause.Returning{Columns: []clause.Column{
			{Name: "id"},
			{Name: "stock"},
		}}).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock - ?", quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		}).
		Scan(&result).Error
	if err != nil {
		return err
	}

	if result.ID == uuid.Nil {
		return errno.ErrProductStockInsufficient
	}

	return repo.createStockChangeLog(ctx, productID, -quantity, result.Stock+quantity, model.StockChangeOrder)
}

func (repo *Repository) UpdateProperty(ctx context.Context, p *model.Product) error {
	p.UpdatedAt = time.Now()
	return repo.GetDB(ctx).Model(p).
		Where("id = ?", p.ID).
		Select([]string{"id", "publisher", "created_at"}).
		Updates(p).Error
}

type ListProductsData struct {
	PageNum  int
	PageSize int
}

func (repo *Repository) ListProducts(ctx context.Context, data ListProductsData) ([]*model.Product, int64, error) {
	var products []*model.Product
	var total int64

	baseQuery := repo.GetDB(ctx).Model(&model.Product{}).
		Where("status = ?", model.ProductStatusActive)

	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err

	}

	err := baseQuery.
		Session(&gorm.Session{}).
		Select([]string{"id", "publisher", "name", "price", "status", "created_at"}).
		Offset((data.PageNum - 1) * data.PageSize).
		Limit(data.PageSize).
		Order("created_at DESC").
		Find(&products).Error

	return products, total, err
}

type UpdateProductPropertyData struct {
	ProductID uuid.UUID
	Publisher uuid.UUID
	Data      map[string]interface{}
}

func (repo *Repository) Update(ctx context.Context, data UpdateProductPropertyData) error {
	if len(data.Data) == 0 {
		return nil
	}

	data.Data["updated_at"] = time.Now()

	return repo.GetDB(ctx).Model(&model.Product{}).
		Where("id = ? and publisher = ?", data.ProductID, data.Publisher).
		Updates(data.Data).Error
}
