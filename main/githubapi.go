package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func fetchGitHubRepos(token string, username string, page int) ([]GitHubRepository, error) {
	url := fmt.Sprintf("https://api.github.com/users/%s/repos?page=%d&per_page=4&sort=updated", username, page)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request:", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "TelegramGitHubBot")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send a request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api error:%w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var repos []GitHubRepository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("failed to decode response:", err)
	}
	return repos, nil
}

func fetchGitHubProfile(token string, userGHUsername string) (githubProfile UserGitHubProfile, err error) {
	var url string
	if userGHUsername == "" {
		url = "https://api.github.com/user"
	} else {
		url = fmt.Sprintf("https://api.github.com/users/%s", userGHUsername)
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return UserGitHubProfile{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "TelegramGitHubBot")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return UserGitHubProfile{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserGitHubProfile{}, fmt.Errorf("github api error status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UserGitHubProfile{}, err
	}

	var profile UserGitHubProfile

	if err := json.Unmarshal(body, &profile); err != nil {
		return UserGitHubProfile{}, err
	}
	return profile, nil
}

func exchangeCodeForToken(code string) (string, error) {
	apiURL := "https://github.com/login/oauth/access_token"
	data := url.Values{}
	data.Set("client_id", globalClientID)
	data.Set("client_secret", globalClientSecret)
	data.Set("code", code)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}
