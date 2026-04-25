package memory

import "time"

type Message struct {
	ID          uint   `gorm:"primarykey"`
	Role        string `gorm:"not null"`
	Content     string `gorm:"not null"`
	ContentType string `gorm:"not null;default:text"`
	TokensIn    int
	TokensOut   int
	CreatedAt   time.Time
}

type Summary struct {
	ID        uint   `gorm:"primarykey"`
	Content   string `gorm:"not null"`
	FromTime  time.Time
	ToTime    time.Time
	CreatedAt time.Time
}

type Fact struct {
	ID        uint   `gorm:"primarykey"`
	Content   string `gorm:"not null"`
	Category  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PhotoIndex struct {
	ID          uint   `gorm:"primarykey"`
	Filename    string `gorm:"uniqueIndex;not null"`
	FilePath    string `gorm:"not null"`
	FileType    string `gorm:"not null"` // image or video
	Description string
	FileSize    int64
	ModTime     time.Time
	IndexedAt   time.Time
}

type ToolCallRecord struct {
	ID        uint   `gorm:"primarykey"`
	MessageID uint   `gorm:"index"`
	ToolName  string `gorm:"not null"`
	Input     string
	Output    string
	Status    string `gorm:"not null;default:pending"`
	CreatedAt time.Time
}
