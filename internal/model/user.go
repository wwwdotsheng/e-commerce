package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ConstraintUserName  = "uni_user_user_name"
	ConstraintUserEmail = "uni_user_user_email"
)

type User struct {
	ID        uuid.UUID      `gorm:"primaryKey;type:uuid"`
	UserName  string         `gorm:"column:user_name;uniqueIndex:uni_user_user_name;type:varchar(30);not null"`
	Email     string         `gorm:"column:email;uniqueIndex:uni_user_user_email;type:varchar(30);not null"`
	Password  string         `gorm:"column:password;type:varchar(255);not null"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		u.ID = id
	}
	return nil
}
