package store

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestSecurityTraderUpdatesRejectWrongOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	s := NewTraderStore(db)
	if err = s.initTables(); err != nil {
		t.Fatal(err)
	}
	if err = s.Create(&Trader{ID: "target", UserID: "owner", Name: "target", AIModelID: "m", ExchangeID: "e"}); err != nil {
		t.Fatal(err)
	}
	for name, update := range map[string]func() error{
		"visibility": func() error { return s.UpdateShowInCompetition("attacker", "target", false) },
		"prompt":     func() error { return s.UpdateCustomPrompt("attacker", "target", "malicious", true) },
		"status":     func() error { return s.UpdateStatus("attacker", "target", true) },
		"balance":    func() error { return s.UpdateInitialBalance("attacker", "target", 1) },
		"config":     func() error { return s.Update(&Trader{ID: "target", UserID: "attacker"}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := update(); err == nil {
				t.Fatal("unauthorized update reported success")
			}
		})
	}
}

func TestSecurityTraderDeleteRejectsWrongOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = db.AutoMigrate(&Trader{}, &EquitySnapshot{}); err != nil {
		t.Fatal(err)
	}
	s := NewTraderStore(db)
	if err = s.Create(&Trader{ID: "target", UserID: "owner", Name: "target", AIModelID: "m", ExchangeID: "e"}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&EquitySnapshot{TraderID: "target", TotalEquity: 123}).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.Delete("attacker", "target"); err == nil {
		t.Error("unauthorized delete reported success")
	}
	var count int64
	db.Model(&EquitySnapshot{}).Count(&count)
	if count != 1 {
		t.Fatal("unauthorized delete removed equity history")
	}
	if err = s.Delete("owner", "target"); err != nil {
		t.Fatal(err)
	}
	db.Model(&EquitySnapshot{}).Count(&count)
	if count != 0 {
		t.Fatal("owner delete did not remove equity history")
	}
}

func TestSecurityLegacyUserSessionMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = db.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL, password_hash TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO users (id,email,password_hash) VALUES ('owner','owner@example.test','old')`).Error; err != nil {
		t.Fatal(err)
	}
	s := NewUserStore(db)
	if err = s.initTables(); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err = db.Raw(`SELECT session_version FROM users WHERE id='owner'`).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("migration version: %d", version)
	}
	if err = s.UpdatePassword("owner", "new"); err != nil {
		t.Fatal(err)
	}
	if err = db.Raw(`SELECT session_version FROM users WHERE id='owner'`).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("password reset version: %d", version)
	}
	if err = s.UpdatePassword("missing", "new"); err == nil {
		t.Fatal("missing user reset reported success")
	}
}
