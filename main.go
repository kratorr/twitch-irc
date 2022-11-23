package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"go.uber.org/zap"
)

const (
	CONN_HOST = "irc.chat.twitch.tv:6667"
)

type Message struct {
	tags       string
	source     string
	command    string
	parameters string
}

func newTwitchIRC(nick, password string) *TwitchIRC {
	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar()
	return &TwitchIRC{nick: nick, pass: password, logger: sugar}
}

type TwitchIRC struct {
	address string
	port    int32
	nick    string
	pass    string
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	logger  *zap.SugaredLogger
}

func (twirc *TwitchIRC) init() {
	conn, err := net.Dial("tcp", CONN_HOST)
	if err != nil {
		fmt.Println(err)
	}
	twirc.conn = conn

	reader := bufio.NewReader(twirc.conn)
	writer := bufio.NewWriter(twirc.conn)
	twirc.reader = reader
	twirc.writer = writer
}

func (t *TwitchIRC) write(msg string) {
	_, err := t.writer.WriteString(msg)
	if err != nil {
		fmt.Println(err)
	}
	err = t.writer.Flush()
}

func (t *TwitchIRC) parseIRCMessage(string) {
	msg := &Message{}
	fmt.Println(msg)
}

func (t *TwitchIRC) auth() {
	t.write("CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands\r\n")
	message, _ := t.reader.ReadString('\n')
	if message != "" { // здесь чекнуть ответ от твича
	}
	t.write(fmt.Sprintf("PASS %s\r\nNICK %s\r\n", t.pass, t.nick))
	if message != "nil" { // здесь чекнуть ответ от твича
	}
	t.write(fmt.Sprintf("JOIN #%s\r\n", t.nick))
	if message != "nil" { // здесь чекнуть ответ от твича
	}
}

func (t *TwitchIRC) startLoop() {
	for {

		message, _ := t.reader.ReadString('\n')
		t.parseIRCMessage(message)

		if len(message) > 0 {
			msg := strings.Split(message, " ")
			t.logger.Infoln(msg)

			if msg[0] == "PING" {
				fmt.Println("write", fmt.Sprintf("PONG %s", msg[1]))
				go t.write(fmt.Sprintf("PONG :%s\r\n", strings.Join(msg[1:], " ")))
			} else if len(msg) >= 3 {
				if msg[1] == "JOIN" {
					name := strings.Split(msg[0], "!")
					go t.write(fmt.Sprintf("PRIVMSG #%s :Hello, @%s \r\n", t.nick, name[0][1:]))

				} else {
				}
			}

		}

	}
}

func main() {
	twirc := newTwitchIRC("nick", "oauth:blalbla")

	twirc.init()
	twirc.auth()
	twirc.startLoop()
}
