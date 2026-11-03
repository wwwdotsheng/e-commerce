package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RefundJobType string

const (
	RefundJobTypeCreate RefundJobType = "create_refund"
	RefundJobTypeQuery  RefundJobType = "query_refund"
)

type RefundJob struct {
	ID             uuid.UUID     `gorm:"column:id;primaryKey;type:uuid"`
	RefundID       uuid.UUID     `gorm:"column:refund_id;type:uuid;not null;uniqueIndex:uni_refund_jobs_refund_id"`
	JobType        RefundJobType `gorm:"column:job_type;type:varchar(20);not null"`
	NextRunAt      time.Time     `gorm:"column:next_run_at;type:timestamptz;not null;index:idx_refund_jobs_next_run"`
	CreateAttempts int           `gorm:"column:create_attempts;not null;default:0"`
	QueryAttempts  int           `gorm:"column:query_attempts;not null;default:0"`
	LastError      string        `gorm:"column:last_error;type:varchar(500);not null;default:''"`
	CreatedAt      time.Time     `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time     `gorm:"column:updated_at;autoUpdateTime"`
}

func (j *RefundJob) TableName() string {
	return "refund_jobs"
}

func (j *RefundJob) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		j.ID = id
	}
	return nil
}
