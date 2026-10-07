package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/yustianfelix/discord-bot/ai"
	"github.com/yustianfelix/discord-bot/handlers"
	"github.com/yustianfelix/discord-bot/music"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; continuing with environment variables only")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN is not set")
	}

	guildID := os.Getenv("DISCORD_GUILD_ID")
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds

	geminiClient, err := ai.NewGeminiClient(geminiKey)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}
	defer func() {
		if err := geminiClient.Close(); err != nil {
			log.Printf("Gemini client shutdown error: %v", err)
		}
	}()

	player := music.NewPlayer(session)
	slashHandler := handlers.NewSlashCommandHandler(geminiClient, player)

	session.AddHandler(func(s *discordgo.Session, event *discordgo.Ready) {
		log.Printf("Logged in as: %s#%s", event.User.Username, event.User.Discriminator)
	})

	session.AddHandler(slashHandler.HandleInteraction)

	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord session: %v", err)
	}
	defer session.Close()

	if guildID != "" {
		if err := registerCommands(session, guildID); err != nil {
			log.Printf("Failed to register guild commands: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("Discord bot is running. Press Ctrl+C to exit.")
	<-ctx.Done()
	log.Println("Shutdown signal received. Cleaning up...")
	player.StopAll()
	<-time.After(100 * time.Millisecond)
}

func registerCommands(session *discordgo.Session, guildID string) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ask",
			Description: "Ask the AI a question",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "question",
					Description: "Your question or prompt",
					Required:    true,
				},
			},
		},
		{
			Name:        "play",
			Description: "Play audio from a YouTube URL or search query",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "query",
					Description: "YouTube URL or search query",
					Required:    true,
				},
			},
		},
	}

	for _, cmd := range commands {
		_, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, cmd)
		if err != nil {
			return err
		}
	}

	return nil
}
