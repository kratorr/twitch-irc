package twirc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

const (
	CONN_HOST = "irc.chat.twitch.tv:6667"
)

// ignore bots from this list https://api.twitchinsights.net/v1/bots/online

type Message struct {
	tags       string
	source     string
	command    ParsedCommand
	parameters string
}

type ParsedCommand struct {
	command             string
	channel             string
	isCapRequestEnabled bool
}

type BotsOnline struct{}

func NewTwitchIRC(nick, password string) *TwitchIRC {
	return &TwitchIRC{nick: nick, pass: password}
}

type TwitchIRC struct {
	address    string
	port       int32
	nick       string
	pass       string
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	mu         *sync.Mutex
	botsOnline map[string]struct{} // потом убрать нахер отсюда, чисто для уменьшения спама
}

// func (t *TwitchIRC) OnMessage(func(msg string) string) {
// 	t.write()
// }

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
		log.Fatal("Connection error %s", err)
	}
	t.conn = conn
	t.mu = &sync.Mutex{}
	t.reader = bufio.NewReader(t.conn)
	t.writer = bufio.NewWriter(t.conn)
}

func (t *TwitchIRC) write(msg string) {
	t.mu.Lock()
	_, err := t.writer.WriteString(msg)
	if err != nil {
		fmt.Println("Error when try to write", err)
	}
	err = t.writer.Flush()
	t.mu.Unlock()
}

func (t *TwitchIRC) parseTags(rawTag string) {
}

func (t *TwitchIRC) parseSourceComponent(rawSource string) {
	fmt.Println(rawSource)
}

func (t *TwitchIRC) parseIRCMessage(rawMsg string) *Message {
	fmt.Println("RAW_MSG", rawMsg)
	msg := &Message{}

	var rawTagsComponent string
	var rawSourceComponent string

	if rawMsg[0] == '@' {
		endIdx := strings.Index(rawMsg, " ")
		rawTagsComponent = rawMsg[1:endIdx]
		rawMsg = rawMsg[endIdx+1:]
	}

	// Get the source component (nick and host) of the IRC message.
	// The idx should point to the source part; otherwise, it's a PING command
	idx := 0
	if rawMsg[idx] == ':' { // vs_code!vs_code@vs_code.tmi.twitch.tv
		idx++
		endIdx := strings.Index(rawMsg, " ")
		rawSourceComponent = rawMsg[idx:]
		rawMsg = rawMsg[endIdx+1:]
	}
	// Get the command component of the IRC message.
	// fmt.Println(rawMsg)
	endIdx := strings.Index(rawMsg, ":") // Looking for the parameters part of the message.
	if -1 == endIdx {                    // But not all messages include the parameters part.
		endIdx = len(rawMsg)
	}
	idx = 0
	rawCommandComponent := strings.TrimSpace(rawMsg[idx:endIdx])
	msg.command = t.parseCommand(rawCommandComponent)

	t.parseTags(rawTagsComponent)
	t.parseSourceComponent(rawSourceComponent)

	// fmt.Println(msg.command.command, msg.command.channel)

	return msg
}

func (t *TwitchIRC) parseCommand(rawCommandComponent string) ParsedCommand {
	var parsedCommand ParsedCommand
	fmt.Println("rawCommandComponent", rawCommandComponent)
	commandParts := strings.Split(rawCommandComponent, " ")
	switch commandParts[0] {
	case "JOIN":
	case "PART":
	case "NOTICE":
	case "CLEARCHAT":
	case "HOSTTARGET":
	case "WHISPER":
	case "PRIVMSG":
		parsedCommand.command = commandParts[0]
		parsedCommand.channel = commandParts[1]
	case "PING":
		parsedCommand.command = commandParts[0]
	case "CAP":
		parsedCommand.command = commandParts[0]
		if commandParts[2] == "ACK" {
			parsedCommand.isCapRequestEnabled = true
		} else {
			parsedCommand.isCapRequestEnabled = false
		}
		// The parameters part of the messages contains the
		// enabled capabilities.
	case "GLOBALUSERSTATE": // Included only if you request the /commands capability.
		// But it has no meaning without also including the /tags capability.
		parsedCommand.command = commandParts[0]
	case "USERSTATE": // Included only if you request the /commands capability.
	case "ROOMSTATE": // But it has no meaning without also including the /tags capabilities.
		parsedCommand.command = commandParts[0]
		parsedCommand.channel = commandParts[1]
	case "RECONNECT":
		fmt.Println("The Twitch IRC server is about to terminate the connection for maintenance.")
		parsedCommand.command = commandParts[0]
	case "421":
		fmt.Printf("Unsupported IRC command: %s", commandParts[2])
	case "001": // Logged in (successfully authenticated).
		parsedCommand.command = commandParts[0]
		parsedCommand.channel = commandParts[1]
	case "002": // Ignoring all other numeric messages.
	case "003":
	case "004":
	case "353": // Tells you who else is in the chat room you're joining.
	case "366":
	case "372":
	case "375":
	case "376":
		fmt.Println("numeric message: %s", commandParts[0])
	default:
		fmt.Printf("\nUnexpected command: %s", commandParts[0])
	}

	return parsedCommand
}

func (t *TwitchIRC) auth() {
	t.write("CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands\r\n")
	msg, _ := t.reader.ReadString('\n')
	if msg != "" { // здесь чекнуть ответ от твича
		fmt.Println(msg)
	}
	t.write(fmt.Sprintf("PASS %s\r\nNICK %s\r\n", t.pass, t.nick))
	msg, _ = t.reader.ReadString('\n')
	if strings.Contains(msg, "Login authentication failed") { // здесь чекнуть ответ от твича
		log.Fatal("Login authentication failed")
	}
	t.write(fmt.Sprintf("JOIN #%s\r\n", t.nick))
	//message, _ = t.reader.ReadString('\n')
	//if message != "nil" { // здесь чекнуть ответ от твича
	//	fmt.Println(message)
	//}
}

func (t *TwitchIRC) startLoop() {
	for {

		message, _ := t.reader.ReadString('\n')
		t.parseIRCMessage(message)

		// if len(message) > 0 {
		// 	msg := strings.Split(message, " ")
		// 	if msg[0] == "PING" {
		// 		go t.write(fmt.Sprintf("PONG :%s\r\n", strings.Join(msg[1:], " ")))
		// 	} else if len(msg) >= 3 {
		// 		// if msg[1] == "JOIN" {
		// 		// 	name := strings.Split(msg[0], "!")
		// 		// 	go t.write(fmt.Sprintf("PRIVMSG #%s :Hi, @%s \r\n", t.nick, name[0][1:]))

		// 		// } else {
		// 		// }
		// 	}

		//}

	}
}
