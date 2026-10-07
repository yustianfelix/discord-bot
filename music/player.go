package music

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pion/opus"
)

const (
	pcmSampleRate = 48000
	pcmChannels   = 2
	pcmFrameSize  = 960
	pcmFrameBytes = 3840
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

	cmd := exec.CommandContext(ctx, "yt-dlp", "--no-playlist", "-f", "bestaudio/best", "-o", "-", query)
	ffmpeg := exec.CommandContext(ctx,
		"ffmpeg",
		"-i", "pipe:0",
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"pipe:1",
	)

	ytPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create yt-dlp stdout pipe: %w", err)
	}
	ffmpeg.Stdin = ytPipe

	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start yt-dlp: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	if err := ffmpeg.Start(); err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	defer func() {
		if ffmpeg.Process != nil {
			_ = ffmpeg.Process.Kill()
		}
	}()

	encoder, err := opus.NewEncoder(pcmSampleRate, pcmChannels, opus.AppAudio)
	if err != nil {
		return fmt.Errorf("create opus encoder: %w", err)
	}

	reader := bufio.NewReader(ffmpegOut)
	pcmBuffer := make([]byte, 0, 4096)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for len(pcmBuffer) < pcmFrameBytes {
				chunk := make([]byte, 4096)
				n, err := reader.Read(chunk)
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return fmt.Errorf("read ffmpeg pcm stream: %w", err)
				}
				if n == 0 {
					continue
				}
				pcmBuffer = append(pcmBuffer, chunk[:n]...)
			}

			frame := pcmBuffer[:pcmFrameBytes]
			pcmBuffer = pcmBuffer[pcmFrameBytes:]

			int16Samples := make([]int16, len(frame)/2)
			for i := 0; i < len(frame); i += 2 {
				int16Samples[i/2] = int16(binary.LittleEndian.Uint16(frame[i : i+2]))
			}

			encoded := make([]byte, 4096)
			n, err := encoder.Encode(int16Samples, encoded)
			if err != nil {
				return fmt.Errorf("encode pcm to opus: %w", err)
			}
			if n > 0 {
				select {
				case vc.OpusSend <- encoded[:n]:
				case <-time.After(200 * time.Millisecond):
					return nil
				}
			}
		}
	}
}
