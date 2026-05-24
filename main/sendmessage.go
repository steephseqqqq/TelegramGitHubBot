package main

import (
	"fmt"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func sendReposMessage(ctx *th.Context, username string, page int, repos []GitHubRepository, messageID int, chatID telego.ChatID) error {
	caption := fmt.Sprintf(`
		📁 Репозитории пользователя <a href="https://github.com/%s">@%s</a>
	`, username, username)

	var keyboard = tu.InlineKeyboard()
	for _, repo := range repos {
		keyboard.InlineKeyboard = append(
			keyboard.InlineKeyboard,
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(repo.FullName).WithCallbackData(fmt.Sprintf("repo:%s", repo.FullName))),
		)
	}
	emptyRepos := 4 - len(repos)
	if emptyRepos > 0 {
		for i := 1; i <= emptyRepos; i++ {
			keyboard.InlineKeyboard = append(
				keyboard.InlineKeyboard,
				tu.InlineKeyboardRow(tu.InlineKeyboardButton("⚪ Пусто").WithCallbackData("empty")),
			)
		}
	}

	if page > 1 && emptyRepos == 0 {
		keyboard.InlineKeyboard = append(
			keyboard.InlineKeyboard,
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("⏮️").WithCallbackData(fmt.Sprintf("user_repos:%s:%d", username, page-1)),
				tu.InlineKeyboardButton("⏭️").WithCallbackData(fmt.Sprintf("user_repos:%s:%d", username, page+1)),
			),
		)
	} else if page > 1 && emptyRepos > 0 {
		keyboard.InlineKeyboard = append(
			keyboard.InlineKeyboard,
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("⏮️").WithCallbackData(fmt.Sprintf("user_repos:%s:%d", username, page-1)),
			),
		)
	} else if page == 1 && emptyRepos == 0 {
		keyboard.InlineKeyboard = append(
			keyboard.InlineKeyboard,
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton("⏭️").WithCallbackData(fmt.Sprintf("user_repos:%s:%d", username, page+1)),
			),
		)
	}

	keyboard.InlineKeyboard = append(
		keyboard.InlineKeyboard,
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("👤 В профиль").WithCallbackData(fmt.Sprintf("profile:%s", username)),
		),
	)

	_, err := ctx.Bot().EditMessageCaption(
		ctx,
		tu.EditMessageCaption(
			chatID,
			messageID,
			caption,
		).WithReplyMarkup(keyboard).WithParseMode(telego.ModeHTML),
	)

	return err
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

	caption, keyboard := buildProfileUI(profile, bIsMe)
	if err != nil {
		return err
	}

	if gitHubUsername == finderGHUsername {
		bIsMe = true
	}

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
