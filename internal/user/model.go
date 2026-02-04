package user

import "e-commerce/internal/model"

const (
	ConstraintUserName  = "uni_user_user_name"
	ConstraintUserEmail = "uni_user_user_email"
)

type User struct {
	model.Base
	UserName string `gorm:"column:user_name;uniqueIndex:uni_user_user_name;type:varchar(30);not null"`
	Email    string `gorm:"column:email;uniqueIndex:uni_user_user_email;type:varchar(30);not null"`
	Password string `gorm:"column:password;type:varchar(255);not null"`
}
