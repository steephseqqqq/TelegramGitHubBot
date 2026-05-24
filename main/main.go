package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

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
	bh.HandleCallbackQuery(handleCallbackProfile, th.CallbackDataPrefix("profile:"))
	bh.HandleCallbackQuery(handleUserRepos, th.CallbackDataPrefix("user_repos:"))
	//bh.HandleCallbackQuery(handleUpdateData, th.CallbackDataPrefix("update:"))

	if err := bh.Start(); err != nil {
		fmt.Println("Failed to start BotHandler:", err)
		os.Exit(1)
	}
}

func buildProfileUI(profile UserGitHubProfile, bIsMe bool) (string, *telego.InlineKeyboardMarkup) {
	var keyboard *telego.InlineKeyboardMarkup
	reposButton := tu.InlineKeyboardButton("📁 Репозитории").WithCallbackData(fmt.Sprintf("user_repos:%s:1", profile.Login))

	if bIsMe {
		keyboard = tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("A").WithCallbackData("profile:torvalds"),
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

	caption := fmt.Sprintf(`
		👤 Профиль пользователя

		• GitHub: <a href="https://github.com/%s">@%s</a>
		`, profile.Login, profile.Login)

	return caption, keyboard
}
