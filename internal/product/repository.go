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
		db.Clauses(clause.Locking{Strength: string(database.LockUpdate)})
	case database.LockShare:
		db.Clauses(clause.Locking{Strength: string(database.LockShare)})

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
}

// TODO(8)[2026-04-29] 为model.Product添加一个商品库存记录变化表, 标记是什么原因减少的库存
// TODO(3)[2026-04-29]
//  假设这样一个需求, 假如买到一半发现商品没货了怎么办？这时候商家需要紧急调整库存或商品状态
//  好像可以直接调整商品状态为缺货好点，假设商家设置了99999+库存，这样可以直接调整为缺货

// UpdateStock 更新商品
func (repo *Repository) UpdateStock(ctx context.Context, data UpdateStockData) error {
	db := repo.GetDB(ctx).Model(&model.Product{}).Where("id = ? and publisher = ?", data.ProductID, data.Publisher)
	if data.Quantity < 0 {
		db = db.Where("stock >= ?", -data.Quantity)
	}

	var updatedID uuid.UUID
	// TODO(4)[2026-04-29] 这样声明结构体的话会不会在调用方法的时候反复声明导致资源损耗
	type UpdateResult struct {
		ID    uuid.UUID
		Stock int
	}
	result := db.
		// TODO(5)[2026-04-29] 这里为什么只需要updatedID？为什么不是使用&model.product去接受？
		Clauses(clause.Returning{Columns: []clause.Column{{
			Name: "id",
		}}}).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock + ?", data.Quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		}).
		// TODO(6)[2026-04-29] 这里Scan的原理是什么？
		Scan(&updatedID)
	if result.Error != nil {
		return result.Error
	}

	// TODO(7)[2026-04-29] 这里如何分离库存不足和商品不存在的错误
	if result.RowsAffected == 0 {
		return errors.New("update failed: insufficient stock or product not found")
	}
	return nil
}

// DeductStock 下单扣减库存（无 publisher 校验，事务内使用）
func (repo *Repository) DeductStock(ctx context.Context, productID uuid.UUID, quantity int) error {
	result := repo.GetDB(ctx).Model(&model.Product{}).
		Where("id = ? AND stock >= ?", productID, quantity).
		Updates(map[string]interface{}{
			"stock":      gorm.Expr("stock - ?", quantity),
			"version":    gorm.Expr("version + 1"),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errno.ErrProductStockInsufficient
	}
	return nil
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
