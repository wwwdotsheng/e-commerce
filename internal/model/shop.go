package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Shop struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	OwnerID     uuid.UUID `gorm:"column:owner_id;type:uuid;not null;index"`
	Name        string    `gorm:"column:name;type:varchar(128);not null"`
	Description string    `gorm:"column:description;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (s *Shop) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		s.ID = id
	}
	return nil
}
