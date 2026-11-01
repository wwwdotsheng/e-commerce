package user

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ApplicationView struct {
	ID              uuid.UUID        `json:"id"`
	UserName        string           `json:"user_name"`
	Email           string           `json:"email"`
	UserType        model.UserType   `json:"user_type"`
	Status          model.UserStatus `json:"status"`
	SellerName      string           `json:"seller_name"`
	ApplicationNote string           `json:"application_note"`
	ReviewReason    string           `json:"review_reason"`
	ReviewedBy      *uuid.UUID       `json:"reviewed_by"`
	ReviewedAt      *time.Time       `json:"reviewed_at"`
}

func view(u model.User) ApplicationView {
	return ApplicationView{u.ID, u.UserName, u.Email, u.UserType, u.Status, u.SellerName, u.ApplicationNote, u.ReviewReason, u.ReviewedBy, u.ReviewedAt}
}
func (svc *Service) GetAccount(ctx context.Context, id uuid.UUID) (*ApplicationView, error) {
	var u model.User
	if err := svc.repo.GetDB(ctx).First(&u, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}
	v := view(u)
	return &v, nil
}
func (svc *Service) ListSellerApplications(ctx context.Context, status model.UserStatus, page, size int) ([]ApplicationView, int64, error) {
	db := svc.repo.GetDB(ctx).Model(&model.User{}).Where("user_type = ? AND status = ?", model.UserTypeSeller, status)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	if err := db.Order("created_at ASC, id ASC").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	result := make([]ApplicationView, 0, len(users))
	for _, u := range users {
		result = append(result, view(u))
	}
	return result, total, nil
}
func (svc *Service) ReviewSellerApplication(ctx context.Context, admin, id uuid.UUID, decision, reason string) error {
	reason = strings.TrimSpace(reason)
	if (decision != "approve" && decision != "reject") || (decision == "reject" && reason == "") {
		return errno.ErrInvalidParam
	}
	return svc.repo.GetDB(ctx).Transaction(func(tx *gorm.DB) error {
		var reviewer model.User
		if err := tx.First(&reviewer, "id = ?", admin).Error; err != nil {
			return err
		}
		if reviewer.UserType != model.UserTypeAdmin || reviewer.Status != model.UserStatusActive {
			return errno.ErrAuthNotPermission
		}
		var u model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, "id = ? AND user_type = ?", id, model.UserTypeSeller).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return errno.ErrNotFoundRecord
			}
			return err
		}
		if u.Status != model.UserStatusPending {
			return errno.ErrSellerApplicationState
		}
		status := model.UserStatusActive
		if decision == "reject" {
			status = model.UserStatusRejected
		}
		now := time.Now()
		if err := tx.Model(&u).Updates(map[string]interface{}{"status": status, "review_reason": reason, "reviewed_by": admin, "reviewed_at": now}).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO seller_application_reviews (user_id, reviewer_id, decision, reason, seller_name, application_note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)", id, admin, decision, reason, u.SellerName, u.ApplicationNote, now).Error
	})
}
func (svc *Service) ResubmitSellerApplication(ctx context.Context, id uuid.UUID, name, note string) error {
	name = strings.TrimSpace(name)
	note = strings.TrimSpace(note)
	if name == "" || note == "" {
		return errno.ErrInvalidParam
	}
	result := svc.repo.GetDB(ctx).Model(&model.User{}).Where("id = ? AND user_type = ? AND status = ?", id, model.UserTypeSeller, model.UserStatusRejected).Updates(map[string]interface{}{"seller_name": name, "application_note": note, "status": model.UserStatusPending, "review_reason": "", "reviewed_by": nil, "reviewed_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errno.ErrSellerApplicationState
	}
	return nil
}

// AccountProfile exposes only the authenticated account's own balance.
type AccountProfile struct {
	ApplicationView
	Balance decimal.Decimal `json:"balance"`
}

func (svc *Service) GetCurrentAccount(ctx context.Context, id uuid.UUID) (*AccountProfile, error) {
	account, err := svc.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	balance, err := svc.walletRepo.GetBalance(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errno.ErrNotFoundRecord
	}
	if err != nil {
		return nil, err
	}
	return &AccountProfile{ApplicationView: *account, Balance: balance}, nil
}
