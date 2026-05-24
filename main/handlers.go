package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

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

func handleCallbackProfile(ctx *th.Context, query telego.CallbackQuery) error {
	var (
		profile UserGitHubProfile
		err     error
	)
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

	bIsMe := false
	if gitHubUsername == finderGHUsername {
		bIsMe = true
	}
	if bIsMe {
		profile, err = GetUserData(query.From.ID)
	} else {
		profile, err = fetchGitHubProfile(token, gitHubUsername)
	}

	if err != nil {
		fmt.Println("handleCallbackProfile: failed to get user profile:", err)
		return fmt.Errorf("failed to get user profile")
	}

	caption, keyboard := buildProfileUI(profile, bIsMe)
	if bIsMe {
		if _, err = ctx.Bot().EditMessageCaption(
			ctx,
			tu.EditMessageCaption(
				query.Message.GetChat().ChatID(),
				query.Message.GetMessageID(),
				caption,
			).WithReplyMarkup(keyboard).WithParseMode(telego.ModeHTML),
		); err != nil {
			fmt.Println("handleCallbackProfile: failed to edit caption:", err)
		}
	} else {
		ctx.Bot().DeleteMessage(
			ctx,
			tu.Delete(query.Message.GetChat().ChatID(), query.Message.GetMessageID()),
		)

		if _, err = ctx.Bot().SendPhoto(
			ctx,
			tu.Photo(
				query.Message.GetChat().ChatID(),
				tu.FileFromURL(profile.AvatarURL),
			).WithReplyMarkup(keyboard).WithParseMode(telego.ModeHTML).WithCaption(caption),
		); err != nil {
			fmt.Println("handleCallbackProfile: failed to edit caption:", err)
		}
	}
	return err
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

func handleUserRepos(ctx *th.Context, query telego.CallbackQuery) error {
	parts := strings.Split(query.Data, ":")
	if len(parts) != 3 {
		fmt.Println("handleUserRepos: invalid query:", query.Data)
		return fmt.Errorf("invalid query")
	}

	messageID := query.Message.GetMessageID()
	chatID := query.Message.GetChat().ChatID()
	username := parts[1]
	pageStr := parts[2]
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		fmt.Println("handleUserRepos: invalid page number:", err)
		return fmt.Errorf("invalid page number")
	}

	token, err := getUserField(globalDB, query.From.ID, "encrypted_access_token")
	if err != nil {
		fmt.Println("handleUserRepos: failed to get user field:", err)
		return fmt.Errorf("failed to get user field")
	}

	repos, err := fetchGitHubRepos(token, username, page)
	if err != nil {
		fmt.Println("handleUserRepos: failed to fetch repos:", err)
		return fmt.Errorf("failed to get repos")
	}

	if err := sendReposMessage(ctx, username, page, repos, messageID, chatID); err != nil {
		fmt.Println("handleUserRepos: failed to send message:", err)
		return fmt.Errorf("failed to send message")
	}
	return nil
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
