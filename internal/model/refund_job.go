package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefundJobType 表示后台下一步要对第三方执行的动作。
type RefundJobType string

const (
	// RefundJobTypeCreate 发起退款；查询确认上次没处理过时也会切回该类型。
	RefundJobTypeCreate RefundJobType = "create_refund"
	// RefundJobTypeQuery 结果不明，按同一 refund_id 查询确认。
	RefundJobTypeQuery RefundJobType = "query_refund"
)

// RefundJob 退款后台任务：同一退款最多一条活动任务。
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
