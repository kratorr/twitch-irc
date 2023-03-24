package twirc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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
	Tags   map[string]string
	Source SourceComponent
	Command
	Parameters string
}
type SourceComponent struct {
	Nick string
	host string
}

type Command struct {
	CommandName         string
	Channel             string
	Body                string
	isCapRequestEnabled bool
}

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

// ignore bots from this list https://api.twitchinsights.net/v1/bots/online
func (t *TwitchIRC) updateBots() {
	t.botsOnline = make(map[string]struct{}, 0)
	var botsJson map[string]interface{}
	resp, err := http.Get("https://api.twitchinsights.net/v1/bots/online")
	if err != nil {
		panic("Failed to fetch online bots")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	json.Unmarshal(body, &botsJson)
	botsOnlineArr := botsJson["bots"].([]interface{})
	for _, val := range botsOnlineArr {
		nickname := val.([]interface{})[0].(string)
		t.botsOnline[nickname] = struct{}{}
	}
}

func (t *TwitchIRC) Start() {
	t.updateBots()
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

func (t *TwitchIRC) parseIRCMessage(rawMsg string) Message {
	zapLog.Debug(rawMsg)
	msg := Message{}

	var rawTagsComponent string
	var rawSourceComponent string
	var rawParametersComponent string

	if rawMsg[0] == '@' {
		endIdx := strings.Index(rawMsg, " ")
		rawTagsComponent = rawMsg[1:endIdx]
		rawMsg = rawMsg[endIdx+1:]
	}

	// Get the source component (nick and host) of the IRC message.
	// The idx should point to the source part; otherwise, it's a PING command
	idx := 0
	if rawMsg[idx] == ':' {
		idx++
		endIdx := strings.Index(rawMsg, " ")
		rawSourceComponent = rawMsg[idx:]
		rawMsg = rawMsg[endIdx+1:]
	}

	// Get the command component of the IRC message.
	endIdx := strings.Index(rawMsg, ":") // Looking for the parameters part of the message.
	if -1 == endIdx {                    // But not all messages include the parameters part.
		endIdx = len(rawMsg)
	}

	idx = 0

	// rawCommandComponent := strings.TrimSpace(rawMsg[idx:endIdx])

	if endIdx != len(rawMsg) { // Check if the IRC message contains a parameters component.
		idx = endIdx + 1 // Should point to the parameters part of the message.
		msg.Command.Body = rawMsg[idx:]
	}

	if rawTagsComponent != "" { // The IRC message contains tags.
		msg.Tags = t.parseTags(rawTagsComponent)
	}

	msg.Source = t.parseSource(rawSourceComponent)
	msg.Parameters = rawParametersComponent

	// zapLog.Debugf("MSG.COMMAND: %#v", msg.Command)
	// zapLog.Debugf("MSG.SOURCE: %#v", msg.Source)
	// zapLog.Debugf("MSG.PARAMETERS: %#v", msg.Parameters)
	// zapLog.Debugf("MSG.TAG: %#v", msg.Tags)

	return msg
}

func (t *TwitchIRC) parseSource(rawSourceComponent string) SourceComponent {
	sc := SourceComponent{}
	if rawSourceComponent == "" {
		return sc
	} else {
		sourceParts := strings.Split(rawSourceComponent, "!")

		if len(sourceParts) == 2 {
			sc.Nick = sourceParts[0]
			sc.host = sourceParts[1]
		}
	}
	return sc
}

func (t *TwitchIRC) parseCommand(rawCommandComponent string) Command {
	var parsedCommand Command
	commandParts := strings.Split(rawCommandComponent, " ")
	switch commandParts[0] {
	case "JOIN":
		parsedCommand.CommandName = commandParts[0]
	case "PART":
	case "NOTICE":
	case "CLEARCHAT":
	case "HOSTTARGET":
	case "WHISPER":
	case "PRIVMSG":
		parsedCommand.CommandName = commandParts[0]
		parsedCommand.Channel = commandParts[1]
	case "PING":
		parsedCommand.CommandName = commandParts[0]
	case "CAP":
		parsedCommand.CommandName = commandParts[0]
		if commandParts[2] == "ACK" {
			parsedCommand.isCapRequestEnabled = true
		} else {
			parsedCommand.isCapRequestEnabled = false
		}

	case "GLOBALUSERSTATE": // Included only if you request the /commands capability.
		// But it has no meaning without also including the /tags capability.
		parsedCommand.CommandName = commandParts[0]
	case "USERSTATE": // Included only if you request the /commands capability.
	case "ROOMSTATE": // But it has no meaning without also including the /tags capabilities.
		parsedCommand.CommandName = commandParts[0]
		parsedCommand.Channel = commandParts[1]
	case "RECONNECT":
		zapLog.Infoln("The Twitch IRC server is about to terminate the connection for maintenance.")
		parsedCommand.CommandName = commandParts[0]
	case "421":
		zapLog.Infof("Unsupported IRC command: %s", commandParts[2])
	case "001": // Logged in (successfully authenticated).

		parsedCommand.CommandName = commandParts[0]
		parsedCommand.Channel = commandParts[1]
	case "002": // Ignoring all other numeric messages.
	case "003":
	case "004":
	case "353":
	case "366":
	case "372":
	case "375":
	case "376":
	default:

	}

	return parsedCommand
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
			twitchMsg := t.parseIRCMessage(message)
			switch twitchMsg.Command.CommandName {
			case "PING":
				go t.write(fmt.Sprintf("PONG %s\r\n", twitchMsg.Parameters))

			case "JOIN":
				t.Messages <- twitchMsg
			case "PRIVMSG":
				t.Messages <- twitchMsg
			}
		}

	}
}
