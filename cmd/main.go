package main

import twitch "github.com/kratorr/twitch-irc"

func main() {
	twirc := twitch.NewTwitchIRC("nick", "oauth:passs")

	twirc.Start()
}
