package main

import (
	"database/sql"
	"fmt"
	"slices"
	"time"
)

func InitDB() (*sql.DB, error) {
	dsn := "postgres://postgres:Stepa1010_1@localhost:5432/githubbotusers?sslmode=disable"

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("Failed to open db: %w", err)
	}

	db.SetMaxIdleConns(25)
	db.SetMaxOpenConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	return db, nil
}

func ExistUser(db *sql.DB, tgID int64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE telegram_id = $1
		)
	`

	var exists bool
	if err := db.QueryRow(query, tgID).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func SaveGitUser(db *sql.DB, tgID int64, ghID int64, username string, avatarURL string, token string) error {
	query := `
			INSERT INTO users
			(
				telegram_id,
				github_id,
				github_username,
				avatar_url,
				encrypted_access_token
			)
			VALUES ($1,$2,$3,$4,$5)
	`

	if _, err := db.Exec(query, tgID, ghID, username, avatarURL, token); err != nil {
		return err
	}

	return nil
}

func GetUserData(tgID int64) (userProfile UserGitHubProfile, err error) {
	query := `
		SELECT
			github_username,
			avatar_url
		FROM users
		WHERE telegram_id = $1
	`

	var profile UserGitHubProfile
	err = globalDB.QueryRow(query, tgID).Scan(&profile.Login, &profile.AvatarURL)
	if err != nil {
		return UserGitHubProfile{}, err
	}
	return profile, nil
}

func getUserField(db *sql.DB, tgID int64, field string) (string, error) {
	if !slices.Contains(fields, field) {
		return "", fmt.Errorf("Invalid field")
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM users
		WHERE telegram_id = $1
	`, field)

	var userField string

	if err := db.QueryRow(query, tgID).Scan(&userField); err != nil {
		return "", fmt.Errorf("failed to get user token:", err)
	}
	return userField, nil
}
