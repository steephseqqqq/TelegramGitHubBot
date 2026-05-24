package main

type UserGitHubProfile struct {
	ID        int64  `json:"id" db:"github_id"`
	Login     string `json:"login" db:"github_username"`
	AvatarURL string `json:"avatar_url" db:"avatar_url"`
}

type GitHubRepository struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	HTMLURL     string `json:"html_url"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
}
