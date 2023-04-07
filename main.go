package twirc

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"go.uber.org/zap"
)

const (
	CONN_HOST = "irc.chat.twitch.tv:6667"
)

var zapLog *zap.SugaredLogger

func init() {
	debug := true
	var logger *zap.Logger
	// потом как нибудь прикрутить при создании бота логгера, мб вместо инита
	if debug == true {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}

	defer logger.Sync()

	zapLog = logger.Sugar()
}

type Message struct {
	Tags       map[string]string
	Parameters []string
	Command    string
	Prefix     string
	Body       string
}

// type SourceComponent struct {
// 	Nick string
// 	host string
// }

// type Command struct {
// 	CommandName         string
// 	Channel             string
// 	Body                string
// 	isCapRequestEnabled bool
// }

// type Tag struct {
// 	badgeInfo   string
// 	badges      []string
// 	color       string
// 	displayName string
// 	emotes      []string
// 	firsMsg     bool
// 	flags       []string
// 	id          string
// 	mod         bool
// 	subscriber  bool
// 	turbo       bool
// 	userId      string
// 	userType    string
// 	Vip         string
// }

type BotsOnline struct{}

func NewTwitchIRC(nick, password string) *TwitchIRC {
	messages := make(chan Message)
	outputMessages := make(chan string)

	return &TwitchIRC{nick: nick, pass: password, Messages: messages, OutputMessages: outputMessages}
}

type TwitchIRC struct {
	address        string
	port           int32
	nick           string
	pass           string
	conn           net.Conn
	reader         *bufio.Reader
	writer         *bufio.Writer
	mu             *sync.Mutex
	botsOnline     map[string]struct{} // потом убрать нахер отсюда, чисто для уменьшения спама
	cacheUserName  map[string]string
	joinHandler    func(Message) (string, error)
	Messages       chan Message
	OutputMessages chan string
}

func (t *TwitchIRC) Start() {
	t.init()
	t.auth()
	t.startLoop()
}

func (t *TwitchIRC) init() {
	conn, err := net.Dial("tcp", CONN_HOST)
	if err != nil {
		zapLog.Fatal("Connection error %s", err)
	}
	t.conn = conn
	t.mu = &sync.Mutex{}
	t.reader = bufio.NewReader(t.conn)
	t.writer = bufio.NewWriter(t.conn)
}

func (t *TwitchIRC) write(msg string) {
	t.mu.Lock()
	_, err := t.writer.WriteString(msg)

	zapLog.Debugf("Message sent: %s", msg)
	if err != nil {
		fmt.Println("Error when try to write", err)
	}

	err = t.writer.Flush()
	t.mu.Unlock()
}

func (t *TwitchIRC) parseTags(rawTag string) map[string]string {
	msgTag := make(map[string]string)
	if len(rawTag) < 1 {
		return msgTag
	}
	tagElems := strings.Split(rawTag, ";")
	for _, tagElem := range tagElems {
		tagElemArr := strings.Split(tagElem, "=")
		msgTag[tagElemArr[0]] = tagElemArr[1]
	}
	// if _, found := msgTag["display-name"]; found {
	// 	t.cacheUserName[t.sour] := msgTag["display-name"]
	// }
	return msgTag
}

func (t *TwitchIRC) parseIRCMessage(rawMessage string) (Message, error) {
	zapLog.Debug("RAW MSG ", rawMessage)

	tags := make(map[string]string)
	msg := Message{}
	var prefix, command string
	var params []string
	// Проверяем наличие тегов
	if strings.HasPrefix(rawMessage, "@") {
		tagsIndex := strings.Index(rawMessage, " ")
		if tagsIndex == -1 {
			return msg, errors.New("Erorr find tag delimetes") // Ошибка: Нет разделителя между тегами и командой
		}
		tagsRaw := rawMessage[1:tagsIndex]
		rawMessage = rawMessage[tagsIndex+1:]

		tagsList := strings.Split(tagsRaw, ";")
		for _, tag := range tagsList {
			if strings.Contains(tag, "=") {
				keyVal := strings.Split(tag, "=")
				tags[keyVal[0]] = keyVal[1]
			} else {
				tags[tag] = ""
			}
		}
	}
	msg.Tags = tags

	// Проверяем наличие префикса
	if strings.HasPrefix(rawMessage, ":") {
		prefixIndex := strings.Index(rawMessage, " ")
		if prefixIndex == -1 {
			return msg, errors.New("") // Ошибка: Нет разделителя между префиксом и командой
		}
		prefix = rawMessage[1:prefixIndex]
		rawMessage = rawMessage[prefixIndex+1:]
	}

	// Получаем команду
	commandIndex := strings.Index(rawMessage, " ")
	if commandIndex == -1 {
		command = rawMessage
		return msg, errors.New("")
	}
	command = rawMessage[0:commandIndex]
	rawMessage = strings.TrimRight(rawMessage[commandIndex+1:], "\r\n")

	// Получаем параметры
	for _, param := range strings.Split(rawMessage, " ") {
		if strings.HasPrefix(param, ":") {
			params = append(params, strings.TrimRight(param, "\n"))
			break
		} else {
			params = append(params, strings.TrimRight(param, "\n"))
		}
	}

	msg.Body = params[len(params)-1][1:]

	msg.Parameters = params
	msg.Command = command
	msg.Prefix = prefix
	// fmt.Printf("%+v\n", msg)
	return msg, nil
}

func (t *TwitchIRC) auth() {
	t.write("CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands\r\n")
	msg, _ := t.reader.ReadString('\n')
	if msg != "" { // здесь чекнуть ответ от твича
	}
	t.write(fmt.Sprintf("PASS %s\r\nNICK %s\r\n", t.pass, t.nick))
	msg, _ = t.reader.ReadString('\n')
	if strings.Contains(msg, "Login authentication failed") { // здесь чекнуть ответ от твича
		zapLog.Fatal("Login authentication failed")
	}
	t.write(fmt.Sprintf("JOIN #%s\r\n", t.nick))
	message, _ := t.reader.ReadString('\n')
	if message != "nil" { // здесь чекнуть ответ от твича
	}
}

func (t *TwitchIRC) JoinHanler(msg Message) string {
	response, _ := t.joinHandler(msg)
	return response
}

func (t *TwitchIRC) startLoop() {
	go func() {
		for outMsg := range t.OutputMessages {
			zapLog.Debug("From outputmessages channel: %s", outMsg)
			go t.write(fmt.Sprintf("PRIVMSG #%s :%s \r\n", t.nick, outMsg))
		}
	}()

	for {

		message, err := t.reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			panic("Error read messages from socket")
		}

		if len(message) > 0 {
			twitchMsg, err := t.parseIRCMessage(strings.TrimSuffix(message, "\n"))
			if err != nil {
				fmt.Println("Error parse message", err)
			}
			// t.parseIRCMessage(message)
			switch twitchMsg.Command {
			case "PING":
				go t.write(fmt.Sprintf("PONG :%s\r\n", twitchMsg.Body))
			case "JOIN":
				t.Messages <- twitchMsg
			case "PRIVMSG":
				t.Messages <- twitchMsg
			}
		}

	}
}
