package music

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

// Player manages voice connections and playback of queued audio streams.
type Player struct {
	session       *discordgo.Session
	mu            sync.Mutex
	voiceSession  *discordgo.VoiceConnection
	currentCancel context.CancelFunc
}

func NewPlayer(s *discordgo.Session) *Player {
	return &Player{session: s}
}

func (p *Player) Play(ctx context.Context, guildID, channelID, query string) error {
	if p.session == nil {
		return fmt.Errorf("discord session is nil")
	}

	p.mu.Lock()
	if p.currentCancel != nil {
		p.currentCancel()
	}
	p.mu.Unlock()

	vc, err := p.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return fmt.Errorf("join voice channel: %w", err)
	}

	p.mu.Lock()
	p.voiceSession = vc
	p.mu.Unlock()

	stopCtx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.currentCancel = cancel
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.currentCancel = nil
		p.mu.Unlock()
		cancel()
		_ = vc.Disconnect()
		p.mu.Lock()
		if p.voiceSession == vc {
			p.voiceSession = nil
		}
		p.mu.Unlock()
	}()

	return p.streamFromCommand(stopCtx, vc, query)
}

func (p *Player) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.currentCancel != nil {
		p.currentCancel()
		p.currentCancel = nil
	}
	if p.voiceSession != nil {
		_ = p.voiceSession.Disconnect()
		p.voiceSession = nil
	}
}

func (p *Player) streamFromCommand(ctx context.Context, vc *discordgo.VoiceConnection, query string) error {
	if vc == nil {
		return fmt.Errorf("nil voice connection")
	}

	if err := vc.Speaking(true); err != nil {
		return fmt.Errorf("start speaking state: %w", err)
	}
	defer func() { _ = vc.Speaking(false) }()

	target := strings.TrimSpace(query)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") && !strings.HasPrefix(target, "ytsearch") {
		target = fmt.Sprintf("ytsearch:%s", target)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", "--no-playlist", "-f", "bestaudio/best", "-o", "-", target)

	ytPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create yt-dlp stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start yt-dlp: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	opts := *dca.StdEncodeOptions
	opts.RawOutput = true

	encodeSession, err := dca.EncodeMem(ytPipe, &opts)
	if err != nil {
		return fmt.Errorf("create dca encode session: %w", err)
	}
	defer encodeSession.Cleanup()

	done := make(chan error, 1)
	_ = dca.NewStream(encodeSession, vc, done)

	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		if err != nil && err != io.EOF {
			return fmt.Errorf("streaming audio: %w", err)
		}
		return nil
	}
}
