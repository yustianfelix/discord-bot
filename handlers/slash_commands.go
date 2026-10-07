package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/yustianfelix/discord-bot/ai"
	"github.com/yustianfelix/discord-bot/music"
)

// SlashCommandHandler processes Discord slash commands.
type SlashCommandHandler struct {
	ai     *ai.GeminiClient
	player *music.Player
}

func NewSlashCommandHandler(aiClient *ai.GeminiClient, player *music.Player) *SlashCommandHandler {
	return &SlashCommandHandler{ai: aiClient, player: player}
}

func (h *SlashCommandHandler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	commandName := i.ApplicationCommandData().Name
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("Failed to defer response for %s: %v", commandName, err)
		return
	}

	switch commandName {
	case "ask":
		h.handleAsk(s, i)
	case "play":
		h.handlePlay(s, i)
	default:
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Unknown command: %s", commandName),
		})
	}
}

func (h *SlashCommandHandler) handleAsk(s *discordgo.Session, i *discordgo.InteractionCreate) {
	question := i.ApplicationCommandData().Options[0].StringValue()
	if strings.TrimSpace(question) == "" {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Please provide a valid question.",
		})
		return
	}

	response, err := h.ai.Ask(context.Background(), question)
	if err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("I couldn't answer that right now: %v", err),
		})
		return
	}

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: response,
	})
}

func (h *SlashCommandHandler) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	if strings.TrimSpace(query) == "" {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "Please provide a valid YouTube URL or search query.",
		})
		return
	}

	if i.Member == nil || i.Member.VoiceState == nil || i.Member.VoiceState.ChannelID == "" {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "You must be in a voice channel to use /play.",
		})
		return
	}

	channelID := i.Member.VoiceState.ChannelID
	if err := h.player.Play(context.Background(), i.GuildID, channelID, query); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Playback failed: %v", err),
		})
		return
	}

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Playing: %s", query),
	})
}
