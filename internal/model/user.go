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

type UserType string

const (
	UserTypeBuyer  UserType = "buyer"
	UserTypeSeller UserType = "seller"
	UserTypeAdmin  UserType = "admin"
)

type UserStatus string

const (
	UserStatusPending  UserStatus = "pending"
	UserStatusActive   UserStatus = "active"
	UserStatusRejected UserStatus = "rejected"
	UserStatusDisabled UserStatus = "disabled"
)

type User struct {
	UserType        UserType   `gorm:"type:varchar(16);not null;default:buyer"`
	Status          UserStatus `gorm:"type:varchar(16);not null;default:active"`
	SellerName      string     `gorm:"type:varchar(128);not null;default:''"`
	ApplicationNote string     `gorm:"type:varchar(1000);not null;default:''"`
	ReviewReason    string     `gorm:"type:varchar(1000);not null;default:''"`
	ReviewedBy      *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt      *time.Time

	ID        uuid.UUID      `gorm:"primaryKey;type:uuid"`
	UserName  string         `gorm:"column:user_name;uniqueIndex:uni_user_user_name;type:varchar(30);not null"`
	Email     string         `gorm:"column:email;uniqueIndex:uni_user_user_email;type:varchar(254);not null"`
	Password  string         `gorm:"column:password;type:varchar(255);not null"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string {
	return "users"
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
