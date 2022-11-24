package twirc

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
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

func NewTwitchIRC(nick, password string) *TwitchIRC {
	return &TwitchIRC{nick: nick, pass: password}
}

//

type TwitchIRC struct {
	address string
	port    int32
	nick    string
	pass    string
	conn    net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	mu      *sync.Mutex
}

// func (t *TwitchIRC) OnMessage(func(msg string) string) {
// 	t.write()
// }

func (t *TwitchIRC) Start() {
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
		fmt.Println("ETO TUT", err)
	}
	err = t.writer.Flush()
	t.mu.Unlock()
}

func (t *TwitchIRC) parseIRCMessage(rawMsg string) {
	msg := &Message{}
	idx := 0

	var rawTagsComponent string
	var rawSourceComponent string

	if rawMsg[idx] == '@' {
		endIdx := strings.Index(rawMsg, " ")
		rawTagsComponent = rawMsg[1:endIdx]
		rawMsg = rawMsg[endIdx+1:]
	} else {
		return
	}
	idx = 0

	// Get the source component (nick and host) of the IRC message.
	// The idx should point to the source part; otherwise, it's a PING command
	if rawMsg[0] == ':' {
		fmt.Println(rawMsg)
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
	rawCommandComponent := rawMsg[idx-1 : endIdx]
	msg.command = t.parseCommand(rawCommandComponent)

	fmt.Println("CUT_RAW", rawMsg)
	fmt.Println(msg)
	fmt.Println("rawTagsComponent", rawTagsComponent)
	fmt.Println("rawSourceComponent", rawSourceComponent)
	fmt.Println("rawCommandComponent", rawCommandComponent)
}

func (t *TwitchIRC) parseCommand(command string) string {
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

		if len(message) > 0 {
			msg := strings.Split(message, " ")
			if msg[0] == "PING" {
				go t.write(fmt.Sprintf("PONG :%s\r\n", strings.Join(msg[1:], " ")))
			} else if len(msg) >= 3 {
				if msg[1] == "JOIN" {
					name := strings.Split(msg[0], "!")
					go t.write(fmt.Sprintf("PRIVMSG #%s :Hi, @%s \r\n", t.nick, name[0][1:]))

				} else {
				}
			}

		}

	}
}
