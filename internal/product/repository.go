package product

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/errno"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	Price       decimal.Decimal
	Status      *model.ProductStatus
	Stock       int
	SellerID    uuid.UUID
}

func (repo *Repository) CreateProduct(ctx context.Context, data CreateProductData) (*model.Product, error) {
	pStatus := model.ProductStatusInactive
	if data.Status != nil && data.Status.IsValid() {
		pStatus = *data.Status
	}

	p := &model.Product{
		SellerID:    data.SellerID,
		Name:        data.Name,
		Description: data.Description,
		Price:       data.Price,
		Stock:       data.Stock,
		Status:      pStatus,
		Version:     1,
	}
	if err := repo.GetDB(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
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
	SellerID  uuid.UUID
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

// UpdateStock 更新商品库存（校验 seller_id）
// classifyStockUpdateMiss 区分库存更新未命中的两种原因：商品不存在/非本店，还是库存不足。
func classifyStockUpdateMiss(productExists bool) *errno.Errno {
	if !productExists {
		return errno.ErrProductNotFound
	}
	return errno.ErrProductStockInsufficient
}

func (repo *Repository) UpdateStock(ctx context.Context, data UpdateStockData) error {
	db := repo.GetDB(ctx).Model(&model.Product{}).Where("id = ? and seller_id = ?", data.ProductID, data.SellerID)
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
		var exists int64
		if err := repo.GetDB(ctx).Model(&model.Product{}).
			Where("id = ? AND seller_id = ?", data.ProductID, data.SellerID).
			Count(&exists).Error; err != nil {
			return err
		}
		return classifyStockUpdateMiss(exists > 0)
	}

	return repo.createStockChangeLog(ctx, data.ProductID, data.Quantity, result.Stock-data.Quantity, data.Reason)
}

// DeductStock 下单扣减库存（无 seller_id 校验，事务内使用）
func (repo *Repository) DeductStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	var updated model.Product
	// RETURNING populates the model directly. Scan would run a second SELECT
	// with stock >= quantity, incorrectly rejecting the final units after update.
	result := repo.GetDB(ctx).Model(&updated).
		Where("id = ? AND stock >= ?", productID, quantity).
		Clauses(clause.Returning{}).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock - ?", quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errno.ErrProductStockInsufficient
	}
	return repo.createStockChangeLog(ctx, productID, -quantity, updated.Stock+quantity, model.StockChangeOrder)
}

// RestoreOrderStock is used only within the timeout transaction.
func (repo *Repository) RestoreOrderStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	var updated model.Product
	result := repo.GetDB(ctx).Model(&updated).Where("id = ?", productID).
		Clauses(clause.Returning{}).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock + ?", quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return repo.createStockChangeLog(ctx, productID, quantity, updated.Stock-quantity, model.StockChangeTimeout)
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
		Select([]string{"id", "seller_id", "name", "price", "status", "created_at"}).
		Offset((data.PageNum - 1) * data.PageSize).
		Limit(data.PageSize).
		Order("created_at DESC, id DESC").
		Find(&products).Error

	return products, total, err
}

type UpdateProductPropertyData struct {
	ProductID uuid.UUID
	SellerID  uuid.UUID
	Data      map[string]interface{}
}

func (repo *Repository) FindByIDs(ctx context.Context, ids []string) ([]*model.Product, error) {
	var products []*model.Product
	err := repo.GetDB(ctx).
		Where("id IN ?", ids).
		Find(&products).Error
	return products, err
}

func (repo *Repository) FindAll(ctx context.Context) ([]*model.Product, error) {
	var products []*model.Product
	err := repo.GetDB(ctx).
		Find(&products).Error
	return products, err
}

func (repo *Repository) Update(ctx context.Context, data UpdateProductPropertyData) error {
	if len(data.Data) == 0 {
		return nil
	}

	data.Data["updated_at"] = time.Now()

	result := repo.GetDB(ctx).Model(&model.Product{}).
		Where("id = ? and seller_id = ?", data.ProductID, data.SellerID).
		Updates(data.Data)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errno.ErrProductNotFound
	}
	return nil
}
