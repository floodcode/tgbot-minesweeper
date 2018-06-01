package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/floodcode/gosweep"
	"github.com/floodcode/tbf"
	"github.com/floodcode/tgbot"
)

const (
	configPath     = "config.json"
	playGameRegexp = `([0-9]+)\s+([0-9]+)\s+([0-9]+)`
	minMines       = 1
	minSize        = 4
	maxSize        = 8
)

var (
	bot       tbf.TelegramBotFramework
	botConfig BotConfig
	games     = map[int]*gosweep.Minefield{}
)

// BotConfig contains bot's environment variables
type BotConfig struct {
	Token string `json:"token"`
	Delay int    `json:"delay"`
}

// CellCallbackData used to store callback data for each minefield cell
type CellCallbackData struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

func main() {
	configData, err := ioutil.ReadFile(configPath)
	checkError(err)

	err = json.Unmarshal(configData, &botConfig)
	checkError(err)

	bot, err = tbf.New(botConfig.Token)
	checkError(err)

	addRoutes()

	err = bot.Poll(tbf.PollConfig{
		Delay: botConfig.Delay,
	})

	checkError(err)
}

func checkError(e error) {
	if e != nil {
		panic(e)
	}
}

func addRoutes() {
	bot.AddRoute("start", helpAction)
	bot.AddRoute("help", helpAction)
	bot.AddRoute("play", playAction)
	bot.OnCallbackQuery(callbackQueryListener)
}

func helpAction(req tbf.Request) {
	req.QuickMessageMD(fmt.Sprintf(strings.Join([]string{
		"Available commads:",
		"/help - Get this message",
		"/play - Play new game",
	}, "\n")))
}

func playAction(req tbf.Request) {
	game, err := createGame(req)
	if err != nil {
		req.QuickMessageMD(err.Error())
		return
	}

	msg, err := req.SendMessage(tgbot.SendMessageConfig{
		Text:        "New game",
		ReplyMarkup: renderMinefield(game),
	})

	if err != nil {
		return
	}

	games[msg.MessageID] = game
}

func callbackQueryListener(req tbf.CallbackQueryRequest) {
	var cellData CellCallbackData
	err := json.Unmarshal([]byte(req.CallbackQuery.Data), &cellData)
	msg := req.CallbackQuery.Message
	if err != nil || msg == nil {
		return
	}

	game, ok := games[msg.MessageID]
	if !ok {
		req.NoAnswer()
		return
	}

	game.Open(cellData.Row, cellData.Col)

	gameState := game.GetState()
	if gameState == gosweep.GameRunning {
		req.Bot.EditMessageText(tgbot.EditMessageTextConfig{
			ChatID:      tgbot.ChatID(msg.Chat.ID),
			MessageID:   msg.MessageID,
			Text:        "Minesweeper",
			ReplyMarkup: renderMinefield(game),
		})
		return
	}

	var notificationText string
	if gameState == gosweep.GameWin {
		notificationText = "You won!"
	} else if gameState == gosweep.GameLose {
		notificationText = "Game over!"
	}

	if len(notificationText) == 0 {
		req.NoAnswer()
		return
	}

	req.Answer(tgbot.AnswerCallbackQueryConfig{
		Text:      notificationText,
		ShowAlert: true,
	})

	req.Bot.EditMessageText(tgbot.EditMessageTextConfig{
		ChatID:      tgbot.ChatID(msg.Chat.ID),
		MessageID:   msg.MessageID,
		Text:        notificationText,
		ReplyMarkup: renderMinefield(game),
	})
}

func createGame(req tbf.Request) (*gosweep.Minefield, error) {
	req.QuickMessage("Enter minefield width:")
	width, err := strconv.ParseInt(req.WaitNext().Message.Text, 10, 32)
	if err != nil || width < minSize || width > maxSize {
		return nil, fmt.Errorf("Width should be in between `%d` and `%d`", minSize, maxSize)
	}

	req.QuickMessage("Enter minefield height:")
	height, err := strconv.ParseInt(req.WaitNext().Message.Text, 10, 32)
	if err != nil || width < minSize || width > maxSize {
		return nil, fmt.Errorf("Height should be in between `%d` and `%d`", minSize, maxSize)
	}

	req.QuickMessage("Enter mines count:")
	mines, err := strconv.ParseInt(req.WaitNext().Message.Text, 10, 32)
	if err != nil {
		return nil, errors.New("Invalid mines count")
	}

	maxMines := int64(float32(width*height) * 0.8)
	if mines < minMines || mines > maxMines {
		return nil, fmt.Errorf(
			"Max mines count for `%d` by `%d` minefield is `%d`, you entered `%d`",
			width, height, maxMines, mines,
		)
	}

	minefield := gosweep.New(int(width), int(height), int(mines))
	return &minefield, nil
}

func renderMinefield(game *gosweep.Minefield) *tgbot.ReplyMarkup {
	field := game.GetField()
	buttons := make([][]tgbot.InlineKeyboardButton, game.GetHeigth())
	for row := 0; row < game.GetHeigth(); row++ {
		buttons[row] = make([]tgbot.InlineKeyboardButton, game.GetWidth())
		for col := 0; col < game.GetWidth(); col++ {
			cell := field[row][col]
			callbackBytes, _ := json.Marshal(CellCallbackData{
				Row: row,
				Col: col,
			})

			buttons[row][col] = tgbot.InlineKeyboardButton{
				Text:         renderCell(cell),
				CallbackData: string(callbackBytes),
			}
		}
	}

	return tgbot.InlineKeyboardMarkup(buttons)
}

func renderCell(cell gosweep.Cell) string {
	typeChars := map[int]string{
		gosweep.TypeEmpty: " ",
		gosweep.Type1:     "1️⃣",
		gosweep.Type2:     "2️⃣",
		gosweep.Type3:     "3️⃣",
		gosweep.Type4:     "4️⃣",
		gosweep.Type5:     "5️⃣",
		gosweep.Type6:     "6️⃣",
		gosweep.Type7:     "7️⃣",
		gosweep.Type8:     "8️⃣",
		gosweep.TypeMine:  "⚫️",
	}

	stateChars := map[int]string{
		gosweep.StateClosed:  "⬜️",
		gosweep.StateFlagged: "ℹ️",
	}

	if val, ok := stateChars[cell.State]; ok {
		return val
	}

	if val, ok := typeChars[cell.Type]; ok {
		return val
	}

	return fmt.Sprint(cell.Type)
}
