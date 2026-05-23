package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	oauthState = "1"
)

var (
	globalDB           *sql.DB
	globalBot          *telego.Bot
	globalClientSecret string
	globalClientID     string
	fields             = []string{"telegram_id", "github_id", "github_username", "avatar_url", "encrypted_access_token"}
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("Failed to load .env:", err)
		os.Exit(1)
	}

	botToken := os.Getenv("TELEGRAM_APITOKEN")
	globalClientID = os.Getenv("GITHUB_CLIENT_ID")
	globalClientSecret = os.Getenv("GITHUB_CLIENT_SECRET_KEY")

	var err error
	globalDB, err = InitDB()
	if err != nil {
		fmt.Println("Failed to init DB:", err)
		os.Exit(1)
	}
	defer globalDB.Close()

	globalBot, err = telego.NewBot(botToken, telego.WithDefaultLogger(false, true))
	if err != nil {
		fmt.Println("Failed initialization:", err)
		os.Exit(1)
	}

	http.HandleFunc("/auth/callback", handleGitHubAuthorizeCallback)

	go func() {
		fmt.Println("HTTP-Server for GitHub Callbacks started at :8080 ")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal("Failed to start HTTP-Server:", err)
		}
	}()

	var cancel context.CancelFunc
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates, err := globalBot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		fmt.Println("Failed to start LongPolling:", err)
		os.Exit(1)
	}

	bh, err := th.NewBotHandler(globalBot, updates)
	if err != nil {
		fmt.Println("Failed to initializate BotHandler:", err)
		os.Exit(1)
	}
	defer func() { _ = bh.Stop() }()

	bh.HandleMessage(handleStart, th.CommandEqual("start"))
	bh.HandleMessage(handleTextProfile, th.TextEqual("👤 Мой профиль"))
	bh.HandleCallbackQuery(handleShowProfile, th.CallbackDataPrefix("profile:"))
	bh.HandleCallbackQuery(handleViewRepos, th.CallbackDataPrefix("repo_view:"))
	//bh.HandleCallbackQuery(handleReposPage, th.CallbackDataPrefix("repos_page:"))
	//bh.HandleCallbackQuery(handleUpdateData, th.CallbackDataPrefix("update:"))

	if err := bh.Start(); err != nil {
		fmt.Println("Failed to start BotHandler:", err)
		os.Exit(1)
	}
}

func handleStart(ctx *th.Context, message telego.Message) error {
	tgID := message.From.ID
	exists, err := ExistUser(globalDB, message.From.ID)
	if err != nil {
		fmt.Println("Failed to check if user exists:", err)
		return err
	}
	if !exists {
		userState := fmt.Sprintf("tg_%d", tgID)
		authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&state=%s", globalClientID, userState)

		fmt.Printf("[DEBUG] Сгенерированная ссылка: %s\n", authURL)
		fmt.Printf("[DEBUG] Текущий Client ID: '%s'\n", globalClientID)

		_, _ = ctx.Bot().SendMessage(ctx, tu.Message(
			tu.ID(tgID),
			fmt.Sprintf("Здравствуйте, %s!", message.From.FirstName),
		).WithReplyMarkup(tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("Авторизуйтесь через GitHub 🤖").WithURL(authURL),
			),
		)))
		return nil
	}

	_, err = ctx.Bot().SendMessage(ctx, tu.Message(
		tu.ID(tgID),
		"🤖 Добро пожаловать назад! Используйте меню для управления.",
	).WithReplyMarkup(tu.Keyboard(
		tu.KeyboardRow(
			tu.KeyboardButton("👤 Мой профиль"),
		),
	).WithResizeKeyboard()))

	return nil
}

func handleShowProfile(ctx *th.Context, query telego.CallbackQuery) error {
	if !strings.HasPrefix(query.Data, "profile:") {
		fmt.Println("invalid query data:", query.Data)
		return fmt.Errorf("invalid query")
	}

	gitHubUsername := strings.TrimPrefix(query.Data, "profile:")

	token, err := getUserField(globalDB, query.From.ID, "encrypted_access_token")
	if err != nil {
		fmt.Println("failed to get user field:", err)
		return fmt.Errorf("failed to get user profile")
	}
	finderGHUsername, err := getUserField(globalDB, query.From.ID, "github_username")
	if err != nil {
		fmt.Println("failed to get user field:", err)
		return fmt.Errorf("failed to get user profile")
	}

	if err := sendProfileMessage(ctx, gitHubUsername, finderGHUsername, query.From.ID, token); err != nil {
		fmt.Println("failed to send profile message:", err.Error())
		return fmt.Errorf("failed to send profile message")
	}
	return nil
}

func handleTextProfile(ctx *th.Context, message telego.Message) error {
	tgID := message.From.ID

	token, err := getUserField(globalDB, tgID, "encrypted_access_token")
	if err != nil {
		fmt.Println("failed to get token:", err)
		return err
	}

	gitHubUsername, err := getUserField(globalDB, tgID, "github_username")
	if err != nil {
		fmt.Println("failed to get github_username:", err)
		return err
	}

	return sendProfileMessage(ctx, gitHubUsername, gitHubUsername, tgID, token)
}

func sendProfileMessage(ctx *th.Context, gitHubUsername string, finderGHUsername string, tgID int64, token string) error {
	bIsMe := false
	if gitHubUsername == finderGHUsername {
		bIsMe = true
	}

	var (
		profile UserGitHubProfile
		err     error
	)

	if bIsMe {
		profile, err = GetUserData(tgID)
	} else {
		profile, err = fetchGitHubProfile(token, gitHubUsername)
	}

	if err != nil {
		return err
	}

	var keyboard *telego.InlineKeyboardMarkup

	if gitHubUsername == finderGHUsername {
		bIsMe = true
	}

	reposButton := tu.InlineKeyboardButton("📁 Репозитории").WithCallbackData(fmt.Sprintf("repos:%s:1", profile.Login))
	if bIsMe {
		keyboard = tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("A").WithCallbackData("profile:steephseq"),
			),
			tu.InlineKeyboardRow(
				reposButton,
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("🗄️ Обновить данные").WithCallbackData("update_profile_data"),
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("⛔ Выйти из аккаунта").WithCallbackData("logout"),
			))
	} else {
		keyboard = tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				reposButton,
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("🔙 Назад").WithCallbackData("back"),
			),
		)
	}

	caption := fmt.Sprintf(
		`
		👤 Профиль пользователя

		• GitHub: <a href="https://github.com/%s">@%s</a>
		`, profile.Login, profile.Login,
	)

	_, err = globalBot.SendPhoto(
		ctx,
		tu.Photo(
			tu.ID(tgID),
			tu.FileFromURL(profile.AvatarURL),
		).WithCaption(caption).WithParseMode(
			telego.ModeHTML).WithReplyMarkup(keyboard),
	)
	return err
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

func handleViewRepos(ctx *th.Context, query telego.CallbackQuery) error {
	/**pageStr := strings.TrimPrefix(query.Data, "repos_page:")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		_, _ = ctx.Bot().SendMessage(ctx,
			tu.Message(
				query.Message.GetChat().ChatID(),
				"⛔ Не удалось загрузить эту страницу",
			))
		fmt.Println("failed to atoi page number:", err)
		return
	}

	token, err := getUserToken(globalDB, query.From.ID)
	if err != nil {
		fmt.Println(err)
	}

	bot.SendMessage(ctx, tu.Message(query.Message.GetChat().ChatID(), "dgdfg"))
	//repos, err := getUserRepos(token, query.)**/
	return nil
}

func getUserRepos(token string, userID string) ([]GitHubRepository, error) {
	req, err := http.NewRequest("GET", "http://api.github.com/user/repos?per_page=100&sort=updated", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request:%w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "TelegramGitHubBot")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed:%w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status:%s", resp.Status)
	}

	var repos []GitHubRepository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("failed to decode repos json:%w", err)
	}
	return repos, nil
}

func handleGitHubAuthorizeCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if state == "" || code == "" {
		http.Error(w, "Missing state or code", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(state, "tg_") {
		http.Error(w, "Invalid state format", http.StatusBadRequest)
		return
	}

	tgIDStr := strings.TrimPrefix(state, "tg_")
	tgID, err := strconv.ParseInt(tgIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID in state", http.StatusBadRequest)
		return
	}

	token, err := exchangeCodeForToken(code)
	if err != nil {
		http.Error(w, "Exchange token failed", http.StatusInternalServerError)
		return
	}

	profile, err := fetchGitHubProfile(token, "")
	if err != nil {
		http.Error(w, "Failed to fetch GitHub profile", http.StatusInternalServerError)
		return
	}

	if err := SaveGitUser(globalDB, tgID, profile.ID, profile.Login, profile.AvatarURL, token); err != nil {
		http.Error(w, "Database save failed", http.StatusInternalServerError)
		fmt.Println("Database save failed:", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Fprintf(w, "<h3>Успешно!</h3><p>Аккаунт привязан. Можешь закрыть вкладку и вернуться в бота.</p>")
	_, _ = globalBot.SendMessage(
		tgCtx,
		tu.Message(
			tu.ID(tgID), "🎉 Авторизация прошла успешно! Теперь я вижу твой GitHub."))
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
