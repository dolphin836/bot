package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func NewStore(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.AutoMigrate(&Message{}, &Summary{}, &Fact{}, &ToolCallRecord{}, &PhotoIndex{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) DB() *gorm.DB {
	return s.db
}

func (s *Store) AddMessage(msg *Message) error {
	return s.db.Create(msg).Error
}

func (s *Store) GetRecentMessages(limit int) ([]Message, error) {
	var messages []Message
	sub := s.db.Model(&Message{}).Order("id desc").Limit(limit)
	result := s.db.Table("(?) as t", sub).Order("id asc").Find(&messages)
	return messages, result.Error
}

func (s *Store) GetOldestMessages(limit int) ([]Message, error) {
	var messages []Message
	result := s.db.Order("id asc").Limit(limit).Find(&messages)
	return messages, result.Error
}

func (s *Store) MessageCount() (int64, error) {
	var count int64
	result := s.db.Model(&Message{}).Count(&count)
	return count, result.Error
}

func (s *Store) DeleteMessagesBefore(t time.Time) error {
	return s.db.Where("created_at < ?", t).Delete(&Message{}).Error
}

func (s *Store) DeleteMessagesByIDs(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.Delete(&Message{}, ids).Error
}

func (s *Store) AddSummary(summary *Summary) error {
	return s.db.Create(summary).Error
}

func (s *Store) GetRecentSummaries(maxAgeDays int) ([]Summary, error) {
	var summaries []Summary
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	result := s.db.Where("created_at > ?", cutoff).Find(&summaries)
	return summaries, result.Error
}

func (s *Store) AddFact(fact *Fact) error {
	// Deduplicate: skip if exact content already exists
	var count int64
	s.db.Model(&Fact{}).Where("content = ?", fact.Content).Count(&count)
	if count > 0 {
		return nil
	}
	return s.db.Create(fact).Error
}

func (s *Store) GetAllFacts() ([]Fact, error) {
	var facts []Fact
	result := s.db.Order("id asc").Find(&facts)
	return facts, result.Error
}

func (s *Store) DeleteFactsByKeyword(keyword string) error {
	return s.db.Where("content LIKE ?", "%"+keyword+"%").Delete(&Fact{}).Error
}

func (s *Store) AddToolCall(tc *ToolCallRecord) error {
	return s.db.Create(tc).Error
}

func (s *Store) UpdateToolCall(id uint, output string, status string) error {
	return s.db.Model(&ToolCallRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"output": output,
		"status": status,
	}).Error
}

func (s *Store) UpsertPhoto(photo *PhotoIndex) error {
	var existing PhotoIndex
	result := s.db.Where("filename = ?", photo.Filename).First(&existing)
	if result.Error == nil {
		// Update existing
		return s.db.Model(&existing).Updates(map[string]interface{}{
			"description": photo.Description,
			"file_size":   photo.FileSize,
			"mod_time":    photo.ModTime,
			"indexed_at":  photo.IndexedAt,
		}).Error
	}
	return s.db.Create(photo).Error
}

func (s *Store) GetPhotoByFilename(filename string) (*PhotoIndex, error) {
	var photo PhotoIndex
	err := s.db.Where("filename = ?", filename).First(&photo).Error
	if err != nil {
		return nil, err
	}
	return &photo, nil
}

func (s *Store) SearchPhotos(keyword string) ([]PhotoIndex, error) {
	var photos []PhotoIndex
	err := s.db.Where("description LIKE ?", "%"+keyword+"%").Order("mod_time desc").Limit(10).Find(&photos).Error
	return photos, err
}

func (s *Store) GetAllPhotos() ([]PhotoIndex, error) {
	var photos []PhotoIndex
	err := s.db.Order("mod_time desc").Find(&photos).Error
	return photos, err
}

func (s *Store) GetUnindexedPhotos() ([]PhotoIndex, error) {
	var photos []PhotoIndex
	err := s.db.Where("description = '' OR description IS NULL").Find(&photos).Error
	return photos, err
}

func (s *Store) PhotoCount() (int64, error) {
	var count int64
	err := s.db.Model(&PhotoIndex{}).Count(&count).Error
	return count, err
}

func (s *Store) IndexedPhotoCount() (int64, error) {
	var count int64
	err := s.db.Model(&PhotoIndex{}).Where("description != '' AND description IS NOT NULL").Count(&count).Error
	return count, err
}

func (s *Store) ClearAll() error {
	if err := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Message{}).Error; err != nil {
		return err
	}
	if err := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Summary{}).Error; err != nil {
		return err
	}
	if err := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&Fact{}).Error; err != nil {
		return err
	}
	if err := s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ToolCallRecord{}).Error; err != nil {
		return err
	}
	return nil
}
