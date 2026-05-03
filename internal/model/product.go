package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProductStatus string

const (
	ProductStatusActive   ProductStatus = "active"
	ProductStatusInactive ProductStatus = "inactive"
)

func (s ProductStatus) IsValid() bool {
	switch s {
	case ProductStatusActive, ProductStatusInactive:
		return true
	}
	return false
}

type Product struct {
	ID          uuid.UUID     `gorm:"column:id;type:uuid;primaryKey"`
	Publisher   uuid.UUID     `gorm:"column:publisher;type:uuid;not null"`
	Name        string        `gorm:"column:name;type:varchar(255);not null"`
	Description string        `gorm:"column:description;type:text;not null"`
	Price       float64       `gorm:"column:price;type:decimal(16,2);not null"`
	Stock       int           `gorm:"column:stock;not null;default:0;check:stock >= 0"`
	FrozenStock int           `gorm:"column:frozen_stock;not null;default:0;check:stock >= 0"`
	Status      ProductStatus `gorm:"column:status;varchar(16);not null;default:'active'"`
	Version     int           `gorm:"column:version;not null;default:0"`
	CreatedAt   time.Time     `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time     `gorm:"column:updated_at;autoUpdateTime"`
}

func (p *Product) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		p.ID = id
	}
	return nil
}
