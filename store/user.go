package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// UserStore user storage
type UserStore struct {
	db *gorm.DB
}

// User user model
type User struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Email          string    `gorm:"uniqueIndex:idx_users_email;not null" json:"email"`
	PasswordHash   string    `gorm:"column:password_hash;not null" json:"-"`
	SessionVersion uint64    `gorm:"not null;default:0" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// NewUserStore creates a new UserStore
func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) initTables() error {
	// For PostgreSQL with existing table, skip AutoMigrate to avoid index conflicts
	if s.db.Dialector.Name() == "postgres" {
		var tableExists int64
		s.db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'users'`).Scan(&tableExists)

		if tableExists > 0 {
			if err := s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS session_version BIGINT NOT NULL DEFAULT 0`).Error; err != nil {
				return err
			}
			// Table exists - manually ensure all columns exist
			// Core columns (should already exist)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT ''`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			s.db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)

			// Ensure unique index exists on email (don't care about the name)
			var indexExists int64
			s.db.Raw(`
				SELECT COUNT(*) FROM pg_indexes
				WHERE tablename = 'users' AND indexdef LIKE '%email%' AND indexdef LIKE '%UNIQUE%'
			`).Scan(&indexExists)

			if indexExists == 0 {
				s.db.Exec("CREATE UNIQUE INDEX idx_users_email ON users(email)")
			}

			return nil
		}
	}
	return s.db.AutoMigrate(&User{})
}

// Create creates user
func (s *UserStore) Create(user *User) error {
	return s.db.Create(user).Error
}

var ErrAlreadyInitialized = errors.New("system already initialized")

// CreateFirst serializes setup across database connections and processes.
// The count and insert must share a write lock, including when users is empty.
func (s *UserStore) CreateFirst(user *User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var lockSQL string
		switch tx.Dialector.Name() {
		case "sqlite":
			lockSQL = "UPDATE users SET id = id WHERE 1 = 0"
		case "postgres":
			lockSQL = "LOCK TABLE users IN EXCLUSIVE MODE"
		default:
			return fmt.Errorf("unsupported registration database: %s", tx.Dialector.Name())
		}
		if err := tx.Exec(lockSQL).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&User{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrAlreadyInitialized
		}
		return tx.Create(user).Error
	})
}

// GetByEmail gets user by email
func (s *UserStore) GetByEmail(email string) (*User, error) {
	var user User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID gets user by ID
func (s *UserStore) GetByID(userID string) (*User, error) {
	var user User
	err := s.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Count returns the total number of users
func (s *UserStore) Count() (int, error) {
	var count int64
	err := s.db.Model(&User{}).Count(&count).Error
	return int(count), err
}

// GetAllIDs gets all user IDs
func (s *UserStore) GetAllIDs() ([]string, error) {
	var userIDs []string
	err := s.db.Model(&User{}).Order("id").Pluck("id", &userIDs).Error
	return userIDs, err
}

// GetAll returns all users ordered by creation time.
func (s *UserStore) GetAll() ([]User, error) {
	var users []User
	err := s.db.Model(&User{}).Order("created_at").Find(&users).Error
	return users, err
}

// UpdatePassword updates password
func (s *UserStore) UpdatePassword(userID, passwordHash string) error {
	return s.updatePassword(s.db.Model(&User{}).Where("id = ?", userID), passwordHash)
}

// UpdatePasswordIfCurrent prevents a concurrent password reset from being overwritten.
func (s *UserStore) UpdatePasswordIfCurrent(userID, oldHash, passwordHash string) error {
	return s.updatePassword(s.db.Model(&User{}).Where("id = ? AND password_hash = ?", userID, oldHash), passwordHash)
}

func (s *UserStore) updatePassword(query *gorm.DB, passwordHash string) error {
	return requireAffectedRecord(query.Updates(map[string]interface{}{
		"password_hash":   passwordHash,
		"session_version": gorm.Expr("session_version + 1"),
		"updated_at":      time.Now().UTC(),
	}))
}

// RevokeSessions invalidates all current tokens durably, including across restarts.
func (s *UserStore) RevokeSessions(userID string) error {
	return requireAffectedRecord(s.db.Model(&User{}).Where("id = ?", userID).
		UpdateColumn("session_version", gorm.Expr("session_version + 1")))
}

// DeleteAll deletes all users (reset system to uninitialized state)
func (s *UserStore) DeleteAll() error {
	return s.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&User{}).Error
}

// EnsureAdmin ensures admin user exists
func (s *UserStore) EnsureAdmin() error {
	var count int64
	s.db.Model(&User{}).Where("id = ?", "admin").Count(&count)
	if count > 0 {
		return nil
	}
	return s.Create(&User{
		ID:           "admin",
		Email:        "admin@localhost",
		PasswordHash: "",
	})
}
